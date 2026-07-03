// Package rabbitmq contains the AMQP wiring used by the sheet mirror.
package rabbitmq

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

const (
	defaultExchangeSuffix = ".exchange"
	retryDelay            = 30 * time.Second
)

// Topology describes the RabbitMQ resources used by sheet-sync work.
type Topology struct {
	Exchange        string
	MainQueue       string
	RetryQueue      string
	DeadQueue       string
	MainRoutingKey  string
	RetryRoutingKey string
	DeadRoutingKey  string
}

// NewTopology returns deterministic durable queue names for the configured
// sheet-sync queue.
func NewTopology(queue string) Topology {
	return Topology{
		Exchange:        queue + defaultExchangeSuffix,
		MainQueue:       queue,
		RetryQueue:      queue + ".retry",
		DeadQueue:       queue + ".dead",
		MainRoutingKey:  "sheet-sync",
		RetryRoutingKey: "sheet-sync.retry",
		DeadRoutingKey:  "sheet-sync.dead",
	}
}

func declareTopology(ch *amqp.Channel, t Topology) error {
	if err := ch.ExchangeDeclare(t.Exchange, "direct", true, false, false, false, nil); err != nil {
		return fmt.Errorf("declare exchange: %w", err)
	}
	if _, err := ch.QueueDeclare(t.MainQueue, true, false, false, false, nil); err != nil {
		return fmt.Errorf("declare main queue: %w", err)
	}
	if err := ch.QueueBind(t.MainQueue, t.MainRoutingKey, t.Exchange, false, nil); err != nil {
		return fmt.Errorf("bind main queue: %w", err)
	}
	if _, err := ch.QueueDeclare(t.RetryQueue, true, false, false, false, amqp.Table{
		"x-dead-letter-exchange":    t.Exchange,
		"x-dead-letter-routing-key": t.MainRoutingKey,
	}); err != nil {
		return fmt.Errorf("declare retry queue: %w", err)
	}
	if err := ch.QueueBind(t.RetryQueue, t.RetryRoutingKey, t.Exchange, false, nil); err != nil {
		return fmt.Errorf("bind retry queue: %w", err)
	}
	if _, err := ch.QueueDeclare(t.DeadQueue, true, false, false, false, nil); err != nil {
		return fmt.Errorf("declare dead queue: %w", err)
	}
	if err := ch.QueueBind(t.DeadQueue, t.DeadRoutingKey, t.Exchange, false, nil); err != nil {
		return fmt.Errorf("bind dead queue: %w", err)
	}
	return nil
}

// Publisher publishes confirmed persistent sheet-sync messages.
type Publisher struct {
	URL      string
	Queue    string
	Logger   *slog.Logger
	conn     *amqp.Connection
	ch       *amqp.Channel
	topology Topology
}

// NewPublisher constructs a RabbitMQ publisher. It connects lazily on first
// publish so the web server can start even when RabbitMQ is temporarily down.
func NewPublisher(url, queue string, logger *slog.Logger) *Publisher {
	return &Publisher{URL: url, Queue: queue, Logger: logger, topology: NewTopology(queue)}
}

// Publish sends one message and waits for a publisher confirm.
func (p *Publisher) Publish(ctx context.Context, messageID string, body []byte, headers amqp.Table) error {
	ch, err := p.channel()
	if err != nil {
		return err
	}
	confirm, err := ch.PublishWithDeferredConfirmWithContext(ctx, p.topology.Exchange, p.topology.MainRoutingKey, false, false, amqp.Publishing{
		DeliveryMode: amqp.Persistent,
		ContentType:  "application/json",
		MessageId:    messageID,
		Timestamp:    time.Now().UTC(),
		Headers:      headers,
		Body:         body,
	})
	if err != nil {
		p.reset()
		return fmt.Errorf("publish sheet sync: %w", err)
	}
	acked, err := confirm.WaitContext(ctx)
	if err != nil {
		p.reset()
		return fmt.Errorf("confirm sheet sync publish: %w", err)
	}
	if !acked {
		p.reset()
		return errors.New("sheet sync publish was nacked")
	}
	return nil
}

func (p *Publisher) channel() (*amqp.Channel, error) {
	if p.ch != nil && !p.ch.IsClosed() {
		return p.ch, nil
	}
	if p.conn == nil || p.conn.IsClosed() {
		conn, err := amqp.Dial(p.URL)
		if err != nil {
			return nil, fmt.Errorf("connect rabbitmq: %w", err)
		}
		p.conn = conn
	}
	ch, err := p.conn.Channel()
	if err != nil {
		p.reset()
		return nil, fmt.Errorf("open rabbitmq channel: %w", err)
	}
	if err := declareTopology(ch, p.topology); err != nil {
		_ = ch.Close()
		p.reset()
		return nil, err
	}
	if err := ch.Confirm(false); err != nil {
		_ = ch.Close()
		p.reset()
		return nil, fmt.Errorf("enable publisher confirms: %w", err)
	}
	p.ch = ch
	return ch, nil
}

func (p *Publisher) reset() {
	if p.ch != nil {
		_ = p.ch.Close()
		p.ch = nil
	}
	if p.conn != nil {
		_ = p.conn.Close()
		p.conn = nil
	}
}

// Close closes any open publisher connection.
func (p *Publisher) Close() error {
	if p == nil {
		return nil
	}
	var err error
	if p.ch != nil {
		err = p.ch.Close()
	}
	if p.conn != nil {
		if closeErr := p.conn.Close(); err == nil {
			err = closeErr
		}
	}
	return err
}

// DeliveryHandler handles one consumed RabbitMQ delivery.
type DeliveryHandler func(context.Context, amqp.Delivery) error

// Consumer consumes sheet-sync messages and routes failures to retry/dead.
type Consumer struct {
	URL    string
	Queue  string
	Logger *slog.Logger
}

// Run blocks until ctx is cancelled, reconnecting after connection-level
// failures.
func (c *Consumer) Run(ctx context.Context, handler DeliveryHandler) error {
	for ctx.Err() == nil {
		if err := c.runOnce(ctx, handler); err != nil && ctx.Err() == nil {
			if c.Logger != nil {
				c.Logger.Error("rabbitmq consumer", "err", err)
			}
			if err := sleepContext(ctx, 5*time.Second); err != nil {
				return nil
			}
		}
	}
	return nil
}

func (c *Consumer) runOnce(ctx context.Context, handler DeliveryHandler) error {
	topology := NewTopology(c.Queue)
	conn, err := amqp.Dial(c.URL)
	if err != nil {
		return fmt.Errorf("connect rabbitmq: %w", err)
	}
	defer conn.Close()

	consumeCh, err := conn.Channel()
	if err != nil {
		return fmt.Errorf("open consume channel: %w", err)
	}
	defer consumeCh.Close()
	if err := declareTopology(consumeCh, topology); err != nil {
		return err
	}
	if err := consumeCh.Qos(1, 0, false); err != nil {
		return fmt.Errorf("set consumer qos: %w", err)
	}

	publishCh, err := conn.Channel()
	if err != nil {
		return fmt.Errorf("open retry channel: %w", err)
	}
	defer publishCh.Close()
	if err := publishCh.Confirm(false); err != nil {
		return fmt.Errorf("enable retry publisher confirms: %w", err)
	}

	deliveries, err := consumeCh.Consume(topology.MainQueue, "spese-sheet-mirror", false, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("consume sheet sync: %w", err)
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case delivery, ok := <-deliveries:
			if !ok {
				return errors.New("rabbitmq delivery channel closed")
			}
			if err := c.handleDelivery(ctx, publishCh, topology, delivery, handler); err != nil {
				return err
			}
		}
	}
}

// RunOne processes at most one queued message and then returns. If the queue is
// empty it returns delivered=false without waiting.
func (c *Consumer) RunOne(ctx context.Context, handler DeliveryHandler) (delivered bool, err error) {
	count, err := c.RunAvailable(ctx, 1, handler)
	return count > 0, err
}

// RunAvailable processes queued messages until the queue is empty or maxCount
// is reached. A maxCount <= 0 means no limit. It does not wait for future
// messages.
func (c *Consumer) RunAvailable(ctx context.Context, maxCount int, handler DeliveryHandler) (count int, err error) {
	topology := NewTopology(c.Queue)
	conn, err := amqp.Dial(c.URL)
	if err != nil {
		return 0, fmt.Errorf("connect rabbitmq: %w", err)
	}
	defer conn.Close()

	consumeCh, err := conn.Channel()
	if err != nil {
		return 0, fmt.Errorf("open consume channel: %w", err)
	}
	defer consumeCh.Close()
	if err := declareTopology(consumeCh, topology); err != nil {
		return 0, err
	}

	publishCh, err := conn.Channel()
	if err != nil {
		return 0, fmt.Errorf("open retry channel: %w", err)
	}
	defer publishCh.Close()
	if err := publishCh.Confirm(false); err != nil {
		return 0, fmt.Errorf("enable retry publisher confirms: %w", err)
	}

	for maxCount <= 0 || count < maxCount {
		if ctx.Err() != nil {
			return count, ctx.Err()
		}
		delivery, ok, err := consumeCh.Get(topology.MainQueue, false)
		if err != nil {
			return count, fmt.Errorf("get sheet sync: %w", err)
		}
		if !ok {
			return count, nil
		}
		count++
		if err := c.handleDelivery(ctx, publishCh, topology, delivery, handler); err != nil {
			return count, err
		}
	}
	return count, nil
}

func (c *Consumer) handleDelivery(ctx context.Context, publishCh *amqp.Channel, topology Topology, delivery amqp.Delivery, handler DeliveryHandler) error {
	if err := handler(ctx, delivery); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			_ = delivery.Nack(false, true)
			return err
		}
		if c.Logger != nil {
			c.Logger.Error("sheet sync failed", "message_id", delivery.MessageId, "err", err)
		}
		if retryErr := c.retryOrDead(ctx, publishCh, topology, delivery, err); retryErr != nil {
			_ = delivery.Nack(false, true)
			return retryErr
		}
		if err := delivery.Ack(false); err != nil {
			return fmt.Errorf("ack failed sheet sync: %w", err)
		}
		return nil
	}
	if err := delivery.Ack(false); err != nil {
		return fmt.Errorf("ack sheet sync: %w", err)
	}
	return nil
}

func (c *Consumer) retryOrDead(ctx context.Context, ch *amqp.Channel, topology Topology, delivery amqp.Delivery, cause error) error {
	attempt := deliveryAttempt(delivery.Headers) + 1
	routingKey := topology.RetryRoutingKey
	expiration := strconv.FormatInt(retryDelay.Milliseconds(), 10)
	if attempt >= 10 {
		routingKey = topology.DeadRoutingKey
		expiration = ""
	}

	headers := copyHeaders(delivery.Headers)
	headers["x-spese-attempt"] = attempt
	headers["x-spese-last-error"] = cause.Error()
	if err := publishWithConfirm(ctx, ch, topology.Exchange, routingKey, amqp.Publishing{
		DeliveryMode:  delivery.DeliveryMode,
		ContentType:   delivery.ContentType,
		MessageId:     delivery.MessageId,
		Timestamp:     time.Now().UTC(),
		CorrelationId: delivery.CorrelationId,
		Headers:       headers,
		Expiration:    expiration,
		Body:          delivery.Body,
	}); err != nil {
		return fmt.Errorf("publish sheet sync retry: %w", err)
	}
	return nil
}

func publishWithConfirm(ctx context.Context, ch *amqp.Channel, exchange, routingKey string, msg amqp.Publishing) error {
	if msg.DeliveryMode == 0 {
		msg.DeliveryMode = amqp.Persistent
	}
	confirm, err := ch.PublishWithDeferredConfirmWithContext(ctx, exchange, routingKey, false, false, msg)
	if err != nil {
		return err
	}
	acked, err := confirm.WaitContext(ctx)
	if err != nil {
		return err
	}
	if !acked {
		return errors.New("rabbitmq publish was nacked")
	}
	return nil
}

func deliveryAttempt(headers amqp.Table) int {
	if headers == nil {
		return 0
	}
	switch v := headers["x-spese-attempt"].(type) {
	case int:
		return v
	case int32:
		return int(v)
	case int64:
		return int(v)
	case string:
		n, _ := strconv.Atoi(v)
		return n
	default:
		return 0
	}
}

func copyHeaders(headers amqp.Table) amqp.Table {
	out := amqp.Table{}
	for k, v := range headers {
		out[k] = v
	}
	return out
}

func sleepContext(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
