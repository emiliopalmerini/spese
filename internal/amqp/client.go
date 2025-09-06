package amqp

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/rabbitmq/amqp091-go"
)

type Client struct {
	conn         *amqp091.Connection
	channel      *amqp091.Channel
	exchangeName string
	queueName    string
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

// PublishExpenseSync publishes an expense sync message
func (c *Client) PublishExpenseSync(ctx context.Context, id, version int64) error {
	msg := NewExpenseSyncMessage(id, version)
	body, err := msg.ToJSON()
	if err != nil {
		return fmt.Errorf("marshal message: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	err = c.channel.PublishWithContext(
		ctx,
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
	if err != nil {
		return fmt.Errorf("publish message: %w", err)
	}

	slog.InfoContext(ctx, "Published expense sync message", 
		"id", id, 
		"version", version,
		"exchange", c.exchangeName,
		"queue", c.queueName)

	return nil
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