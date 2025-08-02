package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/util/teststorage"
)

func main() {
	tstart := time.Now()
	// Create ephemeral in-memory storage
	ts, err := teststorage.NewWithError()
	if err != nil {
		log.Fatal("Failed to create storage")
	}

	// Add some sample data: 0, 5, 10 for http_requests_total
	app := ts.Appender(context.Background())

	metric := labels.FromStrings("__name__", "http_requests_total")
	now := time.Now()

	samples := []float64{0, 5, 10}
	for i, v := range samples {
		tsMillis := now.Add(time.Duration(i) * time.Second).UnixMilli()
		_, err := app.Append(0, metric, tsMillis, v)
		if err != nil {
			log.Fatal(err)
		}
	}

	if err := app.Commit(); err != nil {
		log.Fatal(err)
	}

	// Create PromQL engine
	engine := promql.NewEngine(promql.EngineOpts{
		MaxSamples:    10000,
		Timeout:       5 * time.Second,
		LookbackDelta: 5 * time.Minute,
	})

	queryStr := `http_requests_total`
	start := now
	end := now.Add(3 * time.Second)

	rangeQry, err := engine.NewRangeQuery(
		context.Background(),
		ts,
		nil,
		queryStr,
		start,
		end,
		time.Second,
	)
	if err != nil {
		log.Fatalf("Range query creation error: %v", err)
	}

	res := rangeQry.Exec(context.Background())
	if res.Err != nil {
		log.Fatalf("Range query error: %v", res.Err)
	}

	fmt.Println("Range Query result:", res.Value)

	// Clean up
	ts.Close()
	fmt.Printf("Program execution time: %v\n", time.Since(tstart))
}
