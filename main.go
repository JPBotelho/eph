package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	io_prometheus_client "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/util/teststorage"
)

func main() {
	dataFile := flag.String("src", "", "Location of metrics file")
	queryType := flag.String("type", "instant", "Query type: instant or range")
	queryStr := flag.String("query", "", "PromQL query string")
	startTs := flag.Int64("start", 0, "Start time (UNIX ms) - required for range")
	endTs := flag.Int64("end", 0, "End time (UNIX ms) - required for range")
	instantTs := flag.Int64("time", 0, "Instant query time (UNIX ms) - required for instant")
	step := flag.Int64("step", 0, "Step interval for range queries (in seconds)")

	flag.Parse()

	// Validate
	if *queryStr == "" {
		log.Fatal("Error: --query is required")
	}

	if *dataFile == "" {
		log.Fatal("Error: --src is required")
	}

	switch *queryType {
	case "instant":
		if *instantTs == 0 {
			log.Fatal("Error: --time (UNIX ms) is required for instant queries")
		}
		t := time.UnixMilli(*instantTs)
		fmt.Printf("Instant query:\nQuery: %s\nTime: %s\n", *queryStr, t)

	case "range":
		if *startTs == 0 || *endTs == 0 || *step == 0 {
			log.Fatal("Error: --start, --end (UNIX ms) and --step (seconds) are required for range queries")
		}
		start := time.UnixMilli(*startTs)
		end := time.UnixMilli(*endTs)
		fmt.Printf("Range query:\nQuery: %s\nStart: %s\nEnd: %s\nStep: %ds\n",
			*queryStr, start, end, *step)

	default:
		log.Fatal("Error: --type must be either 'instant' or 'range'")
	}

	tstart := time.Now()
	// Create ephemeral in-memory storage
	ts, err := teststorage.NewWithError()
	if err != nil {
		log.Fatal("Failed to create storage")
	}

	f, _ := os.Open(*dataFile)
	if err != nil {
		log.Fatalf("Error opening metrics file: %v", err)
	}
	defer f.Close()

	cleaned, err := CleanupScrapeFile(f)
	if err != nil {
		panic(err)
	}

	// Parse metrics file
	var parser expfmt.TextParser
	metricFamilies, err := parser.TextToMetricFamilies(bytes.NewReader(cleaned))

	if err != nil {
		log.Fatalf("Error parsing metrics: %v", err)
	}
	app := ts.Appender(context.Background())
	// Iterate over metric families
	for name, mf := range metricFamilies {
		for _, m := range mf.Metric {
			var tsMillis int64
			if m.TimestampMs != nil {
				tsMillis = *m.TimestampMs
			} else {
				// If no explicit timestamp, continue
				fmt.Println("Metric without timestamp")
				continue
			}

			// Convert labels to Prometheus internal type
			lbls := labels.NewBuilder(labels.EmptyLabels()).
				Set("__name__", name)

			for _, lp := range m.Label {
				lbls.Set(lp.GetName(), lp.GetValue())
			}

			lset := lbls.Labels()

			// Handle metric types
			switch mf.GetType() {
			case io_prometheus_client.MetricType_COUNTER:
				_, err = app.Append(0, lset, tsMillis, m.GetCounter().GetValue())
			case io_prometheus_client.MetricType_GAUGE:
				_, err = app.Append(0, lset, tsMillis, m.GetGauge().GetValue())
			case io_prometheus_client.MetricType_UNTYPED:
				_, err = app.Append(0, lset, tsMillis, m.GetUntyped().GetValue())
			case io_prometheus_client.MetricType_SUMMARY:
				_, err = app.Append(0, lset, tsMillis, m.GetSummary().GetSampleSum())
			case io_prometheus_client.MetricType_HISTOGRAM:
				_, err = app.Append(0, lset, tsMillis, m.GetHistogram().GetSampleSum())
			}

			if err != nil {
				log.Printf("Append error for %s: %v", name, err)
			}
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

	if *queryType == "instant" {
		rangeQry, err := engine.NewInstantQuery(
			context.Background(),
			ts,
			nil,
			*queryStr,
			time.UnixMilli(*instantTs),
		)
		if err != nil {
			log.Fatalf("\nInstant query creation error: %v", err)
		}

		res := rangeQry.Exec(context.Background())
		if res.Err != nil {
			log.Fatalf("\nInstant query error: %v", res.Err)
		}

		fmt.Println("\nInstant Query result:", res.Value)
	} else {
		rangeQry, err := engine.NewRangeQuery(
			context.Background(),
			ts,
			nil,
			*queryStr,
			time.UnixMilli(*startTs),
			time.UnixMilli(*endTs),
			time.Duration(*step),
		)
		if err != nil {
			log.Fatalf("\nInstant query creation error: %v", err)
		}

		res := rangeQry.Exec(context.Background())
		if res.Err != nil {
			log.Fatalf("\nInstant query error: %v", res.Err)
		}

		fmt.Println("\nInstant Query result:", res.Value)
	}

	// Clean up
	ts.Close()
	fmt.Printf("\nProgram execution time: %v\n", time.Since(tstart))
}
