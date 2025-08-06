package ingester

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/oklog/ulid/v2"
)

// go run . ingester --target localhost:9182 --id 12345 --o ./out.txt
// Run executes the querier logic
func Run(args []string) {
	fs := flag.NewFlagSet("ingester", flag.ExitOnError)

	scrapeTarget := fs.String("target", "", "Target to scrape")
	scrapeInterval := fs.Int("interval", 7, "Scrape interval")
	duration := fs.Int("d", 30, "Seconds to scrape for")
	id := fs.String("id", "", "Job id")
	output := fs.String("o", "", "Output file")
	keyId := fs.String("keyId", "", "Access key id")
	secretKey := fs.String("secretKey", "", "Secret access key")
	bucket := fs.String("bucket", "", "S3 bucket name")

	endpoint := fs.String("endpoint", "", "R2 endpoint")

	// Parse arguments for this subcommand
	if err := fs.Parse(args); err != nil {
		log.Fatalf("Failed to parse args: %v", err)
	}

	if *output == "" {
		fmt.Println("Outputting to R2...")
		if *keyId == "" {
			log.Fatal("Error: --keyId is required")
		}
		if *endpoint == "" {
			log.Fatal("Error: --endpoint is required")
		}
		if *secretKey == "" {
			log.Fatal("Error: --secretKey is required")
		}

		if *bucket == "" {
			log.Fatal("Error: --bucket is required")
		}

	} else {
		fmt.Printf("Outputting to file %s", *output)
	}

	if *scrapeTarget == "" {
		log.Fatal("Error: --target is required")
	}
	fmt.Printf("Configured to scrape target %s every %d\n", *scrapeTarget, *scrapeInterval)

	if *id == "" {
		log.Println("Job ID not set. Generating...")
		newId := ulid.MustNew(ulid.Timestamp(time.Now()), rand.Reader).String()
		id = &newId

		//log.Fatal("Error: --id is required")
	}

	fmt.Printf("Job will be saved with id %s\n", *id)

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
	//if err := os.WriteFile(*output, []byte(joinWithNewlines(buffer)), 0644); err != nil {
	//	fmt.Printf("Failed to write output file: %v\n", err)
	//	return
	//}
	Upload([]byte(joinWithNewlines(buffer)), *endpoint, *bucket, *keyId, *secretKey, *id)

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

func Upload(content []byte, endpoint string, bucket string, keyId string, secretKey string, jobId string) {
	ctx := context.Background()
	region := "auto"
	// Create static credentials provider
	creds := credentials.NewStaticCredentialsProvider(keyId, secretKey, "")

	// Load AWS config without worrying about the global resolver
	cfg, err := config.LoadDefaultConfig(
		ctx,
		config.WithCredentialsProvider(creds),
		config.WithRegion(region), // "auto" works for R2
	)
	if err != nil {
		log.Fatalf("failed to load AWS config: %v", err)
	}

	// Create S3 client with endpoint override for Cloudflare R2
	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})

	// Upload object
	_, err = client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: &bucket,
		Key:    &jobId,
		Body:   bytes.NewReader(content),
	})
	if err != nil {
		log.Fatalf("failed to upload object: %v", err)
	}

	fmt.Printf("✅ Uploaded %s to bucket %s in region %s\n", jobId, bucket, region)
}
