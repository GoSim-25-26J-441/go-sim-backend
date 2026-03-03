package s3

import (
	"bytes"
	"context"
	"fmt"
	"io"

	"github.com/GoSim-25-26J-441/go-sim-backend/config"
	"github.com/aws/aws-sdk-go-v2/aws"
	awscfg "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// Client wraps the S3 client and bucket name for convenience.
// Modules can use this to perform S3 operations; paths are configured per-module via env.
type Client struct {
	*s3.Client
	Bucket string
}

// NewConnection creates an S3 client from config.
// Returns a Client that wraps the S3 client and bucket name.
// When cfg.Bucket is empty, S3 is considered disabled; callers should check before use.
func NewConnection(ctx context.Context, cfg *config.S3Config) (*Client, error) {
	if cfg == nil || cfg.Bucket == "" {
		return nil, fmt.Errorf("S3 bucket is required (set S3_BUCKET)")
	}

	loadOpts := []func(*awscfg.LoadOptions) error{
		awscfg.WithRegion(cfg.Region),
	}
	if cfg.AccessKeyID != "" && cfg.SecretAccessKey != "" {
		loadOpts = append(loadOpts, awscfg.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(cfg.AccessKeyID, cfg.SecretAccessKey, ""),
		))
	}

	awsCfg, err := awscfg.LoadDefaultConfig(ctx, loadOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	s3Opts := []func(*s3.Options){}
	if cfg.Endpoint != "" {
		s3Opts = append(s3Opts, func(o *s3.Options) {
			o.BaseEndpoint = aws.String(cfg.Endpoint)
			o.UsePathStyle = cfg.ForcePathStyle
		})
	}

	client := s3.NewFromConfig(awsCfg, s3Opts...)

	// Verify connection with HeadBucket
	_, err = client.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: aws.String(cfg.Bucket),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to S3 bucket %q: %w", cfg.Bucket, err)
	}

	return &Client{
		Client: client,
		Bucket: cfg.Bucket,
	}, nil
}

// NewConnectionIfConfigured creates an S3 client when S3 is configured (S3_BUCKET set).
// Returns (nil, nil) when cfg is nil or cfg.Bucket is empty, so callers can skip S3 setup.
func NewConnectionIfConfigured(ctx context.Context, cfg *config.S3Config) (*Client, error) {
	if cfg == nil || cfg.Bucket == "" {
		return nil, nil
	}
	return NewConnection(ctx, cfg)
}

// PutObject uploads the given bytes to S3 under the provided key within this client's bucket.
func (c *Client) PutObject(ctx context.Context, key string, data []byte) error {
	if c == nil || c.Client == nil {
		return fmt.Errorf("s3 client is not initialized")
	}

	_, err := c.Client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(c.Bucket),
		Key:    aws.String(key),
		Body:   bytes.NewReader(data),
	})
	if err != nil {
		return fmt.Errorf("failed to put object to S3 (bucket=%s, key=%s): %w", c.Bucket, key, err)
	}
	return nil
}

// GetObject downloads the object for the given key from this client's bucket.
func (c *Client) GetObject(ctx context.Context, key string) ([]byte, error) {
	if c == nil || c.Client == nil {
		return nil, fmt.Errorf("s3 client is not initialized")
	}

	out, err := c.Client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(c.Bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get object from S3 (bucket=%s, key=%s): %w", c.Bucket, key, err)
	}
	defer out.Body.Close()

	data, err := io.ReadAll(out.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read S3 object body (bucket=%s, key=%s): %w", c.Bucket, key, err)
	}
	return data, nil
}
