package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"

	"aarvix-nexus/internal/jobs"
)

// One direct exchange; every worker owns a durable queue bound with its own ID as routing key.
// This is what makes "smart worker selection" possible: the scheduler decides who gets a job.
const Exchange = "nexus.jobs"

func WorkerQueue(id string) string { return "nexus.worker." + id }

func Dial(url string) (*amqp.Connection, error) {
	var err error
	for i := 0; i < 30; i++ {
		var c *amqp.Connection
		if c, err = amqp.Dial(url); err == nil {
			return c, nil
		}
		time.Sleep(2 * time.Second)
	}
	return nil, err
}

func Declare(ch *amqp.Channel) error {
	return ch.ExchangeDeclare(Exchange, "direct", true, false, false, false, nil)
}

// Queues for vanished workers expire after 1h unused; undelivered messages after 10min.
// Stale messages are harmless anyway thanks to fencing tokens.
func DeclareWorkerQueue(ch *amqp.Channel, id string) (amqp.Queue, error) {
	q, err := ch.QueueDeclare(WorkerQueue(id), true, false, false, false,
		amqp.Table{"x-expires": int32(3600000), "x-message-ttl": int32(600000)})
	if err != nil {
		return q, err
	}
	return q, ch.QueueBind(q.Name, id, Exchange, false, nil)
}

type Publisher struct {
	url  string
	mu   sync.Mutex
	conn *amqp.Connection
	ch   *amqp.Channel
}

func NewPublisher(url string) *Publisher { return &Publisher{url: url} }

func (p *Publisher) ensure() error {
	if p.conn != nil && !p.conn.IsClosed() && p.ch != nil && !p.ch.IsClosed() {
		return nil
	}
	if p.conn != nil && !p.conn.IsClosed() {
		p.conn.Close()
	}
	c, err := amqp.Dial(p.url)
	if err != nil {
		return err
	}
	ch, err := c.Channel()
	if err != nil {
		c.Close()
		return err
	}
	if err := Declare(ch); err != nil {
		c.Close()
		return err
	}
	if err := ch.Confirm(false); err != nil { // publisher confirms
		c.Close()
		return err
	}
	p.conn, p.ch = c, ch
	return nil
}

func (p *Publisher) Publish(ctx context.Context, workerID string, m jobs.Message) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := p.ensure(); err != nil {
		return err
	}
	body, err := json.Marshal(m)
	if err != nil {
		return err
	}
	conf, err := p.ch.PublishWithDeferredConfirmWithContext(ctx, Exchange, workerID, false, false, amqp.Publishing{
		ContentType: "application/json", DeliveryMode: amqp.Persistent, MessageId: m.JobID, Body: body,
	})
	if err != nil {
		return err
	}
	ok, err := conf.WaitContext(ctx)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("broker nacked message for job %s", m.JobID)
	}
	return nil
}
