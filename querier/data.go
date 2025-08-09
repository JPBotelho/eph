package querier

import (
	"context"
	"io"
	"log"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
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
