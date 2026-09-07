// Command ingest-load measures the async movie-creation pipeline of
// re-api-books end-to-end: POST /api/v1/movies (publish to RabbitMQ) ->
// movies-service consumer -> GET /api/v1/movies/status/{correlationId}
// (until completed/failed).
//
// This is the equivalent, for this project's actual stack (RabbitMQ +
// MongoDB, not Redis Streams/TimescaleDB), of "Fase 2: Testes de Volume de
// Dados" from the personal load-testing roteiro this tool was written for
// — see docs/performance/README.md for the full write-up and results.
//
// A plain HTTP load test (vegeta et al.) can't measure this: POST only
// enqueues a job and returns 202 immediately, so "requests/sec accepted"
// says nothing about how fast movies actually get created. This tool
// measures both halves, on separate schedules so they don't interfere:
//
//  1. Publish phase: fires all N POSTs with bounded concurrency (-c) as
//     fast as api-gateway accepts them, recording each correlation_id and
//     the time it was sent.
//  2. Drain phase: polls the status of every still-pending correlation_id
//     once per tick (-drain-tick), not continuously per item — polling
//     "per worker, as fast as possible" (the obvious first design) turned
//     out to itself become a confound: at high concurrency the polling
//     load competed with the real creation traffic for the same
//     gRPC/Mongo path, making the measurement partly a test of the
//     polling mechanism rather than of movies-service's consumer. Ticking
//     the whole pending set together keeps status-check load constant
//     regardless of -c.
//
// Usage:
//
//	go run . -url http://localhost:8080 -n 500 -c 20
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"sort"
	"sync"
	"time"
)

type item struct {
	correlationID  string
	sentAt         time.Time
	publishLatency time.Duration
	publishErr     error

	completedAt time.Time
	status      string // "completed", "failed", "timeout", "" (still pending / publish failed)
}

func main() {
	baseURL := flag.String("url", "http://localhost:8080", "api-gateway base URL")
	n := flag.Int("n", 500, "number of movies to create")
	concurrency := flag.Int("c", 20, "concurrent publishers")
	doDrain := flag.Bool("drain", true, "poll GetMovieStatus per item after publishing (see package doc: this itself adds load that scales with n — pass -drain=false and sample MongoDB's document count externally instead for large n)")
	drainTick := flag.Duration("drain-tick", 20*time.Millisecond, "interval between rounds of status checks")
	drainConcurrency := flag.Int("drain-concurrency", 30, "concurrent status checks per drain round")
	drainTimeout := flag.Duration("drain-timeout", 60*time.Second, "give up on remaining pending items after this long")
	flag.Parse()

	client := &http.Client{Timeout: 10 * time.Second}
	items := make([]*item, *n)

	fmt.Printf("=== ingest-load: publicando %d filmes (concorrência %d) ===\n", *n, *concurrency)
	publishAll(client, *baseURL, items, *concurrency)

	if *doDrain {
		fmt.Println("=== esvaziando a fila: consultando status até tudo terminar ou dar timeout ===")
		drain(client, *baseURL, items, *drainTick, *drainConcurrency, *drainTimeout)
	}

	report(items, *n, *concurrency)
}

func publishAll(client *http.Client, baseURL string, items []*item, concurrency int) {
	var wg sync.WaitGroup
	sem := make(chan struct{}, concurrency)

	body, _ := json.Marshal(map[string]string{"title": "Ingest Load Test", "year": "2020"})

	for i := range items {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int) {
			defer wg.Done()
			defer func() { <-sem }()

			t0 := time.Now()
			resp, err := client.Post(baseURL+"/api/v1/movies", "application/json", bytes.NewReader(body))
			latency := time.Since(t0)

			if err != nil {
				items[i] = &item{sentAt: t0, publishLatency: latency, publishErr: err}
				return
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusAccepted {
				items[i] = &item{sentAt: t0, publishLatency: latency, publishErr: fmt.Errorf("status %d", resp.StatusCode)}
				return
			}

			var created struct {
				CorrelationID string `json:"correlation_id"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
				items[i] = &item{sentAt: t0, publishLatency: latency, publishErr: err}
				return
			}

			items[i] = &item{sentAt: t0, publishLatency: latency, correlationID: created.CorrelationID}
		}(i)
	}
	wg.Wait()
}

// drain polls the status of every item that hasn't reached a terminal
// state yet, once per tick, with bounded concurrency shared across the
// whole batch (not per item) — see the package doc for why.
func drain(client *http.Client, baseURL string, items []*item, tick time.Duration, concurrency int, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	sem := make(chan struct{}, concurrency)

	for {
		var pending []*item
		for _, it := range items {
			if it.correlationID != "" && it.status == "" {
				pending = append(pending, it)
			}
		}
		if len(pending) == 0 {
			return
		}
		if time.Now().After(deadline) {
			for _, it := range pending {
				it.status = "timeout"
			}
			return
		}

		var wg sync.WaitGroup
		for _, it := range pending {
			wg.Add(1)
			sem <- struct{}{}
			go func(it *item) {
				defer wg.Done()
				defer func() { <-sem }()

				resp, err := client.Get(baseURL + "/api/v1/movies/status/" + it.correlationID)
				if err != nil {
					return // try again next tick
				}
				defer resp.Body.Close()

				switch resp.StatusCode {
				case http.StatusOK:
					it.completedAt = time.Now()
					it.status = "completed"
				case http.StatusUnprocessableEntity:
					it.completedAt = time.Now()
					it.status = "failed"
				}
			}(it)
		}
		wg.Wait()
		time.Sleep(tick)
	}
}

func report(items []*item, n, concurrency int) {
	var publishLatencies, e2eLatencies []time.Duration
	var publishErrors, completed, failed, timeouts int
	var firstSent, lastCompleted time.Time

	for _, it := range items {
		if it == nil || it.publishErr != nil {
			publishErrors++
			continue
		}
		publishLatencies = append(publishLatencies, it.publishLatency)
		if firstSent.IsZero() || it.sentAt.Before(firstSent) {
			firstSent = it.sentAt
		}

		switch it.status {
		case "completed":
			completed++
			e2eLatencies = append(e2eLatencies, it.completedAt.Sub(it.sentAt))
			if it.completedAt.After(lastCompleted) {
				lastCompleted = it.completedAt
			}
		case "failed":
			failed++
		case "timeout":
			timeouts++
		}
	}

	fmt.Printf("\n=== ingest-load: n=%d concurrency=%d ===\n\n", n, concurrency)

	fmt.Println("-- Publish (POST /api/v1/movies -> 202) --")
	fmt.Printf("Accepted: %d/%d (%d publish errors)\n", len(publishLatencies), n, publishErrors)
	printLatencyStats(publishLatencies)

	fmt.Println("\n-- End-to-end (POST enviado -> GetMovieStatus reporta completed) --")
	fmt.Printf("Completed: %d, Failed: %d, Timeout: %d\n", completed, failed, timeouts)
	if completed > 0 && lastCompleted.After(firstSent) {
		drainWall := lastCompleted.Sub(firstSent)
		fmt.Printf("Drain time (do primeiro POST ao último completed): %s\n", drainWall.Round(time.Millisecond))
		fmt.Printf("Throughput sustentado: %.1f completions/s (completed / drain time)\n", float64(completed)/drainWall.Seconds())
	}
	printLatencyStats(e2eLatencies)
}

func printLatencyStats(latencies []time.Duration) {
	if len(latencies) == 0 {
		fmt.Println("Latencies: (nenhuma amostra)")
		return
	}
	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })

	pct := func(p float64) time.Duration {
		idx := int(p * float64(len(latencies)-1))
		return latencies[idx]
	}

	var sum time.Duration
	for _, l := range latencies {
		sum += l
	}
	mean := sum / time.Duration(len(latencies))

	fmt.Printf(
		"Latencies: mean=%s p50=%s p95=%s p99=%s max=%s\n",
		mean.Round(time.Millisecond), pct(0.50).Round(time.Millisecond),
		pct(0.95).Round(time.Millisecond), pct(0.99).Round(time.Millisecond),
		latencies[len(latencies)-1].Round(time.Millisecond),
	)
}
