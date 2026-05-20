package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/joho/godotenv"
)

func main() {
	err := godotenv.Load("../../.env")
	if err != nil {
		err = godotenv.Load(".env")
		if err != nil {
			log.Fatalf("Error loading .env file: %v", err)
		}
	}

	region := os.Getenv("AWS_DEFAULT_REGION")
	bucket := os.Getenv("AWS_BUCKET")
	accessKey := os.Getenv("AWS_ACCESS_KEY_ID")
	secretKey := os.Getenv("AWS_SECRET_ACCESS_KEY")

	if region == "" || bucket == "" || accessKey == "" || secretKey == "" {
		log.Fatal("Missing AWS configuration in environment variables")
	}

	cfg, err := config.LoadDefaultConfig(context.TODO(),
		config.WithRegion(region),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKey, secretKey, "")),
	)
	if err != nil {
		log.Fatalf("Unable to load SDK config: %v", err)
	}

	client := s3.NewFromConfig(cfg)

	// Create Bucket
	createInput := &s3.CreateBucketInput{
		Bucket: aws.String(bucket),
	}

	// For regions other than us-east-1, we need to specify a LocationConstraint
	if region != "us-east-1" {
		createInput.CreateBucketConfiguration = &types.CreateBucketConfiguration{
			LocationConstraint: types.BucketLocationConstraint(region),
		}
	}

	_, err = client.CreateBucket(context.TODO(), createInput)
	if err != nil {
		// Ignore if bucket already exists
		log.Printf("CreateBucket warning/error: %v\nNote: If it says BucketAlreadyOwnedByYou, it is fine.\n", err)
	} else {
		fmt.Printf("Successfully created S3 bucket: %s in region %s\n", bucket, region)
	}
}
