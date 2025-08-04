package ingester

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

// go run . ingester --target localhost:9182 --id 12345 --o ./out.txt
// Run executes the querier logic
func Run(args []string) {
	fs := flag.NewFlagSet("ingester", flag.ExitOnError)

	scrapeTarget := fs.String("target", "", "Target to scrape")
	scrapeInterval := fs.Int("interval", 7, "Scrape interval")
	duration := fs.Int("d", 300, "Seconds to scrape for")
	id := fs.String("id", "", "Job id")
	output := fs.String("o", "", "Output file")

	// Parse arguments for this subcommand
	if err := fs.Parse(args); err != nil {
		log.Fatalf("Failed to parse args: %v", err)
	}

	// Validate
	if *scrapeTarget == "" {
		log.Fatal("Error: --target is required")
	}
	fmt.Printf("Configured to scrape target %s every %d\n", *scrapeTarget, *scrapeInterval)

	if *id == "" {
		log.Fatal("Error: --id is required")
	}

	fmt.Printf("Job will be saved with id %s\n", *id)

	if *output == "" {
		log.Fatal("Error: --o is required")
	}

	fmt.Println("Validated inputs.")

	startTime := time.Now()
	var buffer []string

	// buffer = str[]
	for int(time.Since(startTime).Seconds()) < *duration {
		fmt.Printf("Next scrape in %d seconds\n", *scrapeInterval)
		time.Sleep(time.Duration(*scrapeInterval) * time.Second)
		// Fetch scrapeTarget
		resp, err := http.Get(fmt.Sprintf("http://%s/metrics", *scrapeTarget))
		if err != nil {
			fmt.Printf("Failed to fetch %s: %v\n", *scrapeTarget, err)
			continue
		}

		defer resp.Body.Close()

		rawMetrics, err := io.ReadAll(resp.Body)
		if err != nil {
			panic(err)
		}

		processedMetrics := AddTimestamp(rawMetrics)

		resp.Body.Close()

		// Append to buffer
		buffer = append(buffer, string(processedMetrics))
	}

	// Flush buffer to output file
	if err := os.WriteFile(*output, []byte(joinWithNewlines(buffer)), 0644); err != nil {
		fmt.Printf("Failed to write output file: %v\n", err)
		return
	}

	fmt.Printf("Scraping complete. Output saved to %s\n", *output)
	// flust buffer to output file

}

func joinWithNewlines(lines []string) string {
	result := ""
	for _, line := range lines {
		result += line + "\n"
	}
	return result
}

// AddTimestamp takes Prometheus metrics in []byte form and appends a timestamp
// (in milliseconds) to every metric line that is not a comment (#...).
func AddTimestamp(metrics []byte) []byte {
	var buffer bytes.Buffer
	nowMillis := time.Now().UnixMilli()

	reader := bufio.NewReader(bytes.NewReader(metrics))
	for {
		line, err := reader.ReadString('\n')
		if err != nil && err != io.EOF {
			panic(err)
		}

		trimmed := strings.TrimSpace(line)
		if trimmed != "" && !strings.HasPrefix(trimmed, "#") {
			buffer.WriteString(fmt.Sprintf("%s %d\n", trimmed, nowMillis))
		}

		if err == io.EOF {
			break
		}
	}

	return buffer.Bytes()
}
