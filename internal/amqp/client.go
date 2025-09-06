package amqp

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"sync/atomic"
	"time"

	"github.com/rabbitmq/amqp091-go"
)

const (
	// Circuit breaker states
	StateClosed   = 0
	StateOpen     = 1
	StateHalfOpen = 2
	
	// Circuit breaker configuration
	maxFailures     = 5
	openTimeout     = 30 * time.Second
	maxRetries      = 3
	baseRetryDelay  = 1 * time.Second
)

type Client struct {
	conn         *amqp091.Connection
	channel      *amqp091.Channel
	exchangeName string
	queueName    string
	url          string
	
	// Circuit breaker state
	failureCount int64
	lastFailure  time.Time
	state        int32 // 0 = closed, 1 = open, 2 = half-open
}

func NewClient(url, exchangeName, queueName string) (*Client, error) {
	conn, err := amqp091.Dial(url)
	if err != nil {
		return nil, fmt.Errorf("dial AMQP: %w", err)
	}

	channel, err := conn.Channel()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("open channel: %w", err)
	}

	client := &Client{
		conn:         conn,
		channel:      channel,
		exchangeName: exchangeName,
		queueName:    queueName,
		url:          url,
	}

	if err := client.setup(); err != nil {
		client.Close()
		return nil, fmt.Errorf("setup exchange and queue: %w", err)
	}

	return client, nil
}

func (c *Client) setup() error {
	// Declare exchange
	err := c.channel.ExchangeDeclare(
		c.exchangeName, // name
		"direct",       // type
		true,           // durable
		false,          // auto-deleted
		false,          // internal
		false,          // no-wait
		nil,            // arguments
	)
	if err != nil {
		return fmt.Errorf("declare exchange: %w", err)
	}

	// Declare queue
	_, err = c.channel.QueueDeclare(
		c.queueName, // name
		true,        // durable
		false,       // delete when unused
		false,       // exclusive
		false,       // no-wait
		nil,         // arguments
	)
	if err != nil {
		return fmt.Errorf("declare queue: %w", err)
	}

	// Bind queue to exchange
	err = c.channel.QueueBind(
		c.queueName,    // queue name
		c.queueName,    // routing key (same as queue name for direct exchange)
		c.exchangeName, // exchange
		false,
		nil,
	)
	if err != nil {
		return fmt.Errorf("bind queue: %w", err)
	}

	return nil
}

// isCircuitOpen checks if the circuit breaker is in open state
func (c *Client) isCircuitOpen() bool {
	state := atomic.LoadInt32(&c.state)
	if state == StateOpen {
		// Check if we should transition to half-open
		if time.Since(c.lastFailure) > openTimeout {
			atomic.CompareAndSwapInt32(&c.state, StateOpen, StateHalfOpen)
			return false
		}
		return true
	}
	return false
}

// recordSuccess resets circuit breaker on successful operation
func (c *Client) recordSuccess() {
	atomic.StoreInt64(&c.failureCount, 0)
	atomic.StoreInt32(&c.state, StateClosed)
}

// recordFailure increments failure count and may open circuit
func (c *Client) recordFailure() {
	c.lastFailure = time.Now()
	count := atomic.AddInt64(&c.failureCount, 1)
	if count >= maxFailures {
		atomic.StoreInt32(&c.state, StateOpen)
	}
}

// exponentialBackoff calculates delay for retry attempt
func exponentialBackoff(attempt int) time.Duration {
	if attempt > 10 {
		attempt = 10 // cap at 10 to prevent overflow
	}
	delay := time.Duration(math.Pow(2, float64(attempt))) * baseRetryDelay
	if delay > 30*time.Second {
		delay = 30 * time.Second // max delay of 30 seconds
	}
	return delay
}

// reconnect attempts to re-establish AMQP connection with retry logic
func (c *Client) reconnect(ctx context.Context) error {
	for attempt := 0; attempt < maxRetries; attempt++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		slog.InfoContext(ctx, "Attempting to reconnect to AMQP", "attempt", attempt+1)
		
		conn, err := amqp091.Dial(c.url)
		if err != nil {
			delay := exponentialBackoff(attempt)
			slog.WarnContext(ctx, "AMQP reconnection failed, retrying", 
				"error", err, "attempt", attempt+1, "retry_delay", delay)
			
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
				continue
			}
		}

		channel, err := conn.Channel()
		if err != nil {
			conn.Close()
			delay := exponentialBackoff(attempt)
			slog.WarnContext(ctx, "AMQP channel creation failed, retrying", 
				"error", err, "attempt", attempt+1, "retry_delay", delay)
			
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
				continue
			}
		}

		// Close old connection if it exists
		if c.conn != nil {
			c.conn.Close()
		}
		if c.channel != nil {
			c.channel.Close()
		}

		c.conn = conn
		c.channel = channel

		// Re-setup exchange and queue
		if err := c.setup(); err != nil {
			conn.Close()
			channel.Close()
			delay := exponentialBackoff(attempt)
			slog.WarnContext(ctx, "AMQP setup failed, retrying", 
				"error", err, "attempt", attempt+1, "retry_delay", delay)
			
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
				continue
			}
		}

		slog.InfoContext(ctx, "Successfully reconnected to AMQP", "attempt", attempt+1)
		return nil
	}

	return fmt.Errorf("failed to reconnect after %d attempts", maxRetries)
}

// PublishExpenseSync publishes an expense sync message with circuit breaker and retry
func (c *Client) PublishExpenseSync(ctx context.Context, id, version int64) error {
	// Check circuit breaker
	if c.isCircuitOpen() {
		return fmt.Errorf("circuit breaker is open, skipping publish")
	}

	msg := NewExpenseSyncMessage(id, version)
	body, err := msg.ToJSON()
	if err != nil {
		return fmt.Errorf("marshal message: %w", err)
	}

	// Retry logic with exponential backoff
	for attempt := 0; attempt < maxRetries; attempt++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		publishCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		
		err = c.channel.PublishWithContext(
			publishCtx,
			c.exchangeName, // exchange
			c.queueName,    // routing key
			false,          // mandatory
			false,          // immediate
			amqp091.Publishing{
				ContentType:  "application/json",
				DeliveryMode: amqp091.Persistent, // make message persistent
				Timestamp:    time.Now(),
				Body:         body,
			},
		)
		cancel()
		
		if err == nil {
			c.recordSuccess()
			slog.InfoContext(ctx, "Published expense sync message", 
				"id", id, 
				"version", version,
				"exchange", c.exchangeName,
				"queue", c.queueName,
				"attempt", attempt+1)
			return nil
		}

		// Check if it's a connection error that requires reconnection
		if isConnectionError(err) {
			slog.WarnContext(ctx, "AMQP connection error, attempting reconnection", 
				"error", err, "attempt", attempt+1)
			
			if reconnectErr := c.reconnect(ctx); reconnectErr != nil {
				slog.ErrorContext(ctx, "Failed to reconnect", "error", reconnectErr)
				c.recordFailure()
				
				if attempt < maxRetries-1 {
					delay := exponentialBackoff(attempt)
					select {
					case <-ctx.Done():
						return ctx.Err()
					case <-time.After(delay):
						continue
					}
				}
			} else {
				// Reconnection successful, retry the publish immediately
				continue
			}
		} else {
			// Non-connection error, record failure and retry with backoff
			c.recordFailure()
			slog.WarnContext(ctx, "Failed to publish message, retrying", 
				"error", err, "attempt", attempt+1, "id", id)
			
			if attempt < maxRetries-1 {
				delay := exponentialBackoff(attempt)
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(delay):
					continue
				}
			}
		}
	}

	c.recordFailure()
	return fmt.Errorf("failed to publish message after %d attempts: %w", maxRetries, err)
}

// isConnectionError checks if the error indicates a connection problem
func isConnectionError(err error) bool {
	if err == nil {
		return false
	}
	
	// Check for common AMQP connection errors
	errStr := err.Error()
	return strings.Contains(errStr, "connection") || 
		   strings.Contains(errStr, "closed") || 
		   strings.Contains(errStr, "EOF") ||
		   strings.Contains(errStr, "broken pipe") ||
		   strings.Contains(errStr, "use of closed network connection")
}

// ConsumeExpenseSync consumes expense sync messages
func (c *Client) ConsumeExpenseSync(ctx context.Context, handler func(*ExpenseSyncMessage) error) error {
	msgs, err := c.channel.Consume(
		c.queueName, // queue
		"",          // consumer
		false,       // auto-ack (we want manual ack)
		false,       // exclusive
		false,       // no-local
		false,       // no-wait
		nil,         // args
	)
	if err != nil {
		return fmt.Errorf("start consuming: %w", err)
	}

	slog.InfoContext(ctx, "Started consuming expense sync messages", "queue", c.queueName)

	for {
		select {
		case <-ctx.Done():
			slog.InfoContext(ctx, "Stopping message consumption", "reason", ctx.Err())
			return ctx.Err()
		case delivery, ok := <-msgs:
			if !ok {
				return fmt.Errorf("message channel closed")
			}

			msg, err := ExpenseSyncMessageFromJSON(delivery.Body)
			if err != nil {
				slog.ErrorContext(ctx, "Failed to unmarshal message", "error", err)
				delivery.Nack(false, false) // reject and don't requeue
				continue
			}

			slog.InfoContext(ctx, "Processing expense sync message", 
				"id", msg.ID, 
				"version", msg.Version)

			if err := handler(msg); err != nil {
				slog.ErrorContext(ctx, "Failed to handle message", 
					"error", err, 
					"id", msg.ID, 
					"version", msg.Version)
				delivery.Nack(false, true) // reject and requeue
				continue
			}

			delivery.Ack(false) // acknowledge successful processing
			slog.InfoContext(ctx, "Successfully processed expense sync message", 
				"id", msg.ID, 
				"version", msg.Version)
		}
	}
}

func (c *Client) Close() error {
	if c.channel != nil {
		c.channel.Close()
	}
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}