package querier

import (
	"context"
	"io"
	"log"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/prometheus/prometheus/tsdb"
)

type FileItem struct {
	Context string
	Name    string
	Size    int64
	Date    time.Time
	Data    []byte
}

func GetFiles(bucket string, client s3.Client) []FileItem {
	input := &s3.ListObjectsV2Input{
		Bucket: aws.String(bucket),
	}

	output, err := client.ListObjectsV2(context.Background(), input)
	if err != nil {
		log.Fatalf("failed to list objects: %v", err)
	}

	var files []FileItem
	for _, obj := range output.Contents {
		files = append(files, FileItem{
			Context: bucket,
			Name:    aws.ToString(obj.Key),
			Size:    *obj.Size,
			Date:    aws.ToTime(obj.LastModified),
		})
	}
	return files
}

// ✅ This function fills the Data field of the given FileItem
func DownloadFile(bucket string, file *FileItem, client s3.Client) {
	input := &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(file.Name),
	}

	resp, err := client.GetObject(context.Background(), input)
	if err != nil {
		log.Fatalf("failed to download object %s: %v", file.Name, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Fatalf("failed to read object body: %v", err)
	}
	file.Data = body
}

func GetMetricNames(db tsdb.DB, matcher string) []string {
	// Define a time range to cover the whole database
	// Here, min and max times span the entire TSDB
	minTime := db.Head().MinTime()
	maxTime := db.Head().MaxTime()

	q, err := db.Querier(minTime, maxTime)
	if err != nil {
		log.Printf("error creating querier: %v", err)
		return nil
	}
	defer q.Close()

	metricNames, _, err := q.LabelValues(context.Background(), "__name__", nil)
	if err != nil {
		log.Printf("error fetching metric names: %v", err)
		return nil
	}

	var filtered []string
	for _, name := range metricNames {
		if strings.Contains(name, matcher) {
			filtered = append(filtered, name)
		}
	}

	return filtered
}
