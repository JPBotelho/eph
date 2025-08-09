package querier

import (
	"context"
	"flag"
	"fmt"
	"log"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// go run . querier --src ./out.txt --query go_gc_gogc_percent --time 1754335979103
// Run executes the querier logic
func Run(args []string) {
	fs := flag.NewFlagSet("querier", flag.ExitOnError)

	dataFile := fs.String("src", "", "Location of metrics file")
	keyId := fs.String("keyId", "", "Access key id")
	secretKey := fs.String("secretKey", "", "Secret access key")
	bucket := fs.String("bucket", "", "S3 bucket name")

	//fileFormat := fs.String("format", "sequence", "Metrics file format.")
	queryType := fs.String("type", "instant", "Query type: instant or range")
	queryStr := fs.String("query", "", "PromQL query string")
	startTs := fs.Int64("start", 0, "Start time (UNIX ms) - required for range")
	endTs := fs.Int64("end", 0, "End time (UNIX ms) - required for range")
	instantTs := fs.Int64("time", 0, "Instant query time (UNIX ms) - required for instant")
	step := fs.Int64("step", 0, "Step interval for range queries (in seconds)")
	endpoint := fs.String("endpoint", "", "R2 endpoint")

	client := &s3.Client{}

	// Parse arguments for this subcommand
	if err := fs.Parse(args); err != nil {
		log.Fatalf("Failed to parse args: %v", err)
	}

	// Validate
	if *queryStr == "" {
		log.Fatal("Error: --query is required")
	}

	//mode := ""

	if *dataFile == "" {
		fmt.Println("Reading from R2...")
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

		// Create static credentials provider
		creds := credentials.NewStaticCredentialsProvider(*keyId, *secretKey, "")

		// Load AWS config without worrying about the global resolver
		cfg, err := config.LoadDefaultConfig(
			context.Background(),
			config.WithCredentialsProvider(creds),
			config.WithRegion("auto"), // "auto" works for R2
		)
		if err != nil {
			log.Fatalf("failed to load AWS config: %v", err)
		}

		client = s3.NewFromConfig(cfg, func(o *s3.Options) {
			o.BaseEndpoint = aws.String(*endpoint)
		})

		//mode = "r2"

	} else {
		fmt.Printf("Reading metrics from file %s", *dataFile)
		//mode = "file"
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

	files := GetFiles(*bucket, *client)
	OpenUI(*bucket, files, *client)

	fmt.Printf("\nProgram execution time: %v\n", time.Since(tstart))
}
