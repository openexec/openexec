package uploader

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// mockS3Client captures PutObject calls.
type mockS3Client struct {
	mu       sync.Mutex
	uploaded []string // format: "bucket/key"
	err      error
}

func (m *mockS3Client) PutObject(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.err != nil {
		return nil, m.err
	}

	key := *params.Key
	bucket := *params.Bucket
	m.uploaded = append(m.uploaded, fmt.Sprintf("%s/%s", bucket, key))

	return &s3.PutObjectOutput{}, nil
}

// Stubs for other interface methods required by manager.NewUploader
func (m *mockS3Client) UploadPart(context.Context, *s3.UploadPartInput, ...func(*s3.Options)) (*s3.UploadPartOutput, error) {
	return &s3.UploadPartOutput{}, nil
}
func (m *mockS3Client) CreateMultipartUpload(context.Context, *s3.CreateMultipartUploadInput, ...func(*s3.Options)) (*s3.CreateMultipartUploadOutput, error) {
	return &s3.CreateMultipartUploadOutput{}, nil
}
func (m *mockS3Client) CompleteMultipartUpload(context.Context, *s3.CompleteMultipartUploadInput, ...func(*s3.Options)) (*s3.CompleteMultipartUploadOutput, error) {
	return &s3.CompleteMultipartUploadOutput{}, nil
}
func (m *mockS3Client) AbortMultipartUpload(context.Context, *s3.AbortMultipartUploadInput, ...func(*s3.Options)) (*s3.AbortMultipartUploadOutput, error) {
	return &s3.AbortMultipartUploadOutput{}, nil
}

func TestUploadSession(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "axon-upload-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create some dummy files
	files := []string{
		"stdout.jsonl",
		"stderr.log",
		"meta.json",
		"subdir/extra.txt",
	}

	for _, f := range files {
		path := filepath.Join(tempDir, f)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatalf("failed to create dir: %v", err)
		}
		if err := os.WriteFile(path, []byte("dummy content"), 0644); err != nil {
			t.Fatalf("failed to create file: %v", err)
		}
	}

	mockClient := &mockS3Client{}
	cfg := Config{
		Bucket: "test-bucket",
		Region: "us-east-1",
		Prefix: "evidence/",
	}

	u := &Uploader{
		client: mockClient,
		cfg:    cfg,
	}

	fwuID := "fwu-123"
	timestamp := "20231027-120000"

	err = u.UploadSession(context.Background(), tempDir, fwuID, timestamp)
	if err != nil {
		t.Fatalf("UploadSession failed: %v", err)
	}

	// Verify uploads
	expectedKeys := map[string]bool{
		"evidence/fwu-123/20231027-120000/stdout.jsonl":     false,
		"evidence/fwu-123/20231027-120000/stderr.log":       false,
		"evidence/fwu-123/20231027-120000/meta.json":        false,
		"evidence/fwu-123/20231027-120000/subdir/extra.txt": false,
	}

	mockClient.mu.Lock()
	defer mockClient.mu.Unlock()

	if len(mockClient.uploaded) != len(expectedKeys) {
		t.Errorf("expected %d uploads, got %d", len(expectedKeys), len(mockClient.uploaded))
	}

	for _, uploaded := range mockClient.uploaded {
		// format is "bucket/key"
		// bucket is "test-bucket"
		// key starts after "test-bucket/"
		expectedPrefix := "test-bucket/"
		if len(uploaded) <= len(expectedPrefix) || uploaded[:len(expectedPrefix)] != expectedPrefix {
			t.Errorf("uploaded item %s does not start with bucket name", uploaded)
			continue
		}
		key := uploaded[len(expectedPrefix):]
		if _, ok := expectedKeys[key]; ok {
			expectedKeys[key] = true
		} else {
			t.Errorf("unexpected upload key: %s", key)
		}
	}

	for key, found := range expectedKeys {
		if !found {
			t.Errorf("expected key %s was not uploaded", key)
		}
	}
}

func TestUploadSession_Error(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "axon-upload-error-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a dummy file
	path := filepath.Join(tempDir, "test.txt")
	if err := os.WriteFile(path, []byte("content"), 0644); err != nil {
		t.Fatalf("failed to create file: %v", err)
	}

	mockClient := &mockS3Client{
		err: fmt.Errorf("simulated upload error"),
	}
	cfg := Config{
		Bucket: "test-bucket",
	}

	u := &Uploader{
		client: mockClient,
		cfg:    cfg,
	}

	err = u.UploadSession(context.Background(), tempDir, "fwu", "ts")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}
