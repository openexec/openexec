package uploader

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// Config holds the configuration for the S3 uploader.
type Config struct {
	Bucket   string
	Region   string
	Endpoint string
	Prefix   string
}

// Uploader handles uploading session artifacts to S3.
type Uploader struct {
	client manager.UploadAPIClient
	cfg    Config
}

// New creates a new Uploader.
func New(ctx context.Context, cfg Config) (*Uploader, error) {
	if cfg.Bucket == "" {
		return nil, fmt.Errorf("bucket name is required")
	}
	if cfg.Region == "" {
		cfg.Region = "us-east-1"
	}
	if cfg.Prefix == "" {
		cfg.Prefix = "evidence/"
	}

	awsCfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(cfg.Region))
	if err != nil {
		return nil, fmt.Errorf("unable to load SDK config: %w", err)
	}

	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		if cfg.Endpoint != "" {
			o.BaseEndpoint = aws.String(cfg.Endpoint)
			// Enable path-style addressing which is often required for MinIO/custom endpoints
			o.UsePathStyle = true
		}
	})

	return &Uploader{
		client: client,
		cfg:    cfg,
	}, nil
}

// UploadSession uploads all files from localDir to S3 with the key pattern:
// <Prefix>/<fwuID>/<timestamp>/<relPath>
func (u *Uploader) UploadSession(ctx context.Context, localDir string, fwuID string, timestamp string) error {
	uploader := manager.NewUploader(u.client) //nolint:staticcheck // TODO: migrate to feature/s3/transfermanager

	return filepath.WalkDir(localDir, func(path string, d fs.DirEntry, err error) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		relPath, err := filepath.Rel(localDir, path)
		if err != nil {
			return fmt.Errorf("failed to get relative path for %s: %w", path, err)
		}

		// Normalize path separators to forward slashes for S3 keys
		relPath = filepath.ToSlash(relPath)
		key := fmt.Sprintf("%s%s/%s/%s", u.cfg.Prefix, fwuID, timestamp, relPath)
		// Clean the key to remove double slashes if Prefix ends with one
		// but we want to keep the trailing slash if it's a folder (not here though)
		// Actually filepath.ToSlash might not be enough if we want to ensure standard S3 keys.
		// Let's just ensure we join cleanly.
		// If Prefix is "evidence/", key becomes "evidence/fwuID/timestamp/relPath"

		file, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("failed to open file %s: %w", path, err)
		}
		defer func() { _ = file.Close() }()

		fmt.Printf("Uploading %s to s3://%s/%s\n", path, u.cfg.Bucket, key)

		_, err = uploader.Upload(ctx, &s3.PutObjectInput{ //nolint:staticcheck // TODO: migrate to feature/s3/transfermanager
			Bucket: aws.String(u.cfg.Bucket),
			Key:    aws.String(key),
			Body:   file,
		})
		if err != nil {
			return fmt.Errorf("failed to upload %s: %w", path, err)
		}

		return nil
	})
}
