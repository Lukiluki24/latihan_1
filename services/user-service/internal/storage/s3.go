package storage

import (
	"bytes"
	"context"
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// S3ImageStorage = implementasi usecase.ImageStorage pakai S3 API bawaan SeaweedFS
// (bukan AWS asli, tapi SDK-nya sama persis karena SeaweedFS punya S3-compatible gateway).
type S3ImageStorage struct {
	client *s3.Client
	bucket string
}

func New(client *s3.Client, bucket string) *S3ImageStorage {
	return &S3ImageStorage{client: client, bucket: bucket}
}

// Upload: key dibentuk oleh pemanggil (deterministik, mis. "avatars/<id>/avatar.jpg")
// supaya upload ulang otomatis menimpa file lama — gak perlu hapus manual dulu.
func (s *S3ImageStorage) Upload(ctx context.Context, key string, data []byte, contentType string) error {
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(key),
		Body:        bytes.NewReader(data),
		ContentType: aws.String(contentType),
	})
	if err != nil {
		return fmt.Errorf("s3 put %s: %w", key, err)
	}
	return nil
}

// Get: dipanggil usecase pas ada request GET buat serve gambar balik ke browser.
func (s *S3ImageStorage) Get(ctx context.Context, key string) (io.ReadCloser, string, error) {
	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, "", fmt.Errorf("s3 get %s: %w", key, err)
	}
	contentType := "application/octet-stream"
	if out.ContentType != nil {
		contentType = *out.ContentType
	}
	return out.Body, contentType, nil
}
