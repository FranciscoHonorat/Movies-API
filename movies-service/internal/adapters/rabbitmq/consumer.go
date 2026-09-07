package rabbitmq

import (
	amqp "github.com/rabbitmq/amqp091-go"
)

type Consumer struct {
	channel   *amqp.Channel
	queueName string
}

func NewConsumer(url string, queueName string) (*Consumer, error) {
	conn, err := amqp.Dial(url)
	if err != nil {
		return nil, err
	}

	ch, err := conn.Channel()
	if err != nil {
		return nil, err
	}

	_, err = ch.QueueDeclare(
		queueName,
		true,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		return nil, err
	}

	return &Consumer{
		channel:   ch,
		queueName: queueName,
	}, nil
}

// Consume starts `workers` goroutines pulling deliveries off the same
// AMQP channel and calling handler for each. Multiple goroutines ranging
// over the same Go channel is standard fan-out: each delivery still goes
// to exactly one goroutine, so handler doesn't need its own
// synchronization on account of this — but it does need to be safe to
// call concurrently, since with workers > 1 it now can be. workers <= 1
// falls back to a single goroutine (today's behavior).
func (c *Consumer) Consume(handler func(amqp.Delivery), workers int) error {
	msgs, err := c.channel.Consume(
		c.queueName,
		"",
		true,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		return err
	}

	if workers < 1 {
		workers = 1
	}
	for i := 0; i < workers; i++ {
		go func() {
			for d := range msgs {
				handler(d)
			}
		}()
	}

	return nil
}
