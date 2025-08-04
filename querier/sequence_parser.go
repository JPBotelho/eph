package querier

import (
	"bytes"
	"fmt"
	"log"
	"os"

	io_prometheus_client "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
)

func ParseSequenceFile(app storage.Appender, dataFile string) {
	f, err := os.Open(dataFile)
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
}
