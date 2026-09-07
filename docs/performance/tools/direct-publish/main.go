// Command direct-publish publishes N messages straight to RabbitMQ's
// movies_queue, bypassing api-gateway's HTTP layer entirely.
//
// Why this exists: ingest-load (the sibling tool) publishes via
// POST /api/v1/movies, and no matter how high its concurrency went (up to
// 1000), `rabbitmqctl list_queues` showed messages_ready staying at 0 the
// entire time — movies-service's consumer was draining the queue exactly
// as fast as it filled, i.e. api-gateway's HTTP handling (or the AMQP
// publish-confirm round trip it does per request) was the actual
// throughput ceiling, not the consumer. That makes it impossible to
// answer "does adding consumer workers help?" through the HTTP path: you
// can't observe a queue backlog draining faster if a backlog never forms.
//
// This tool publishes directly to the broker on one channel, without
// waiting for a per-message HTTP round trip, so it can produce messages
// far faster than the consumer can drain them — which is what's needed to
// actually measure how many workers it takes to keep up, and whether
// throughput scales with worker count once there's a real backlog.
//
// Usage:
//
//	go run . -uri amqp://guest:guest@localhost:5672/ -queue movies_queue -n 50000
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/google/uuid"
)

// Matches shared.MoviePublisherMessage — duplicated here rather than
// imported so this tool has no dependency on the rest of the workspace
// (it's a standalone module, see go.mod in this directory).
type moviePublisherMessage struct {
	CorrelationID string `json:"correlation_id"`
	Title         string `json:"title"`
	Year          string `json:"year"`
}

func main() {
	uri := flag.String("uri", "amqp://guest:guest@localhost:5672/", "RabbitMQ URI")
	queue := flag.String("queue", "movies_queue", "queue name")
	n := flag.Int("n", 50000, "number of messages to publish")
	title := flag.String("title", "Ingest Load Test", "title field on every published message (used to find/clean up the resulting documents)")
	flag.Parse()

	conn, err := amqp.Dial(*uri)
	if err != nil {
		log.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	ch, err := conn.Channel()
	if err != nil {
		log.Fatalf("channel: %v", err)
	}
	defer ch.Close()

	if _, err := ch.QueueDeclare(*queue, true, false, false, false, nil); err != nil {
		log.Fatalf("queue declare: %v", err)
	}

	start := time.Now()
	for i := 0; i < *n; i++ {
		body, _ := json.Marshal(moviePublisherMessage{
			CorrelationID: uuid.NewString(),
			Title:         *title,
			Year:          "2020",
		})
		err := ch.Publish("", *queue, false, false, amqp.Publishing{
			ContentType: "application/json",
			Body:        body,
		})
		if err != nil {
			log.Fatalf("publish #%d: %v", i, err)
		}
	}
	elapsed := time.Since(start)

	fmt.Printf("Published %d messages in %s (%.1f msg/s)\n", *n, elapsed.Round(time.Millisecond), float64(*n)/elapsed.Seconds())
}
