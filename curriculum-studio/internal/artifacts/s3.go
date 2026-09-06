package artifacts

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/url"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// S3Store uses a private, pre-provisioned bucket. It never creates a public
// bucket policy. Empty explicit credentials use the standard IAM provider.
type S3Store struct {
	client *minio.Client
	bucket string
}
type S3Options struct{ Endpoint, Bucket, Region, AccessKey, SecretKey string }

func NewS3Store(o S3Options) (*S3Store, error) {
	u, err := url.Parse(o.Endpoint)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return nil, fmt.Errorf("invalid S3 endpoint")
	}
	if o.Bucket == "" {
		return nil, fmt.Errorf("S3 bucket is required")
	}
	if (o.AccessKey == "") != (o.SecretKey == "") {
		return nil, fmt.Errorf("both S3 access and secret keys are required")
	}
	creds := credentials.NewIAM("")
	if o.AccessKey != "" {
		creds = credentials.NewStaticV4(o.AccessKey, o.SecretKey, "")
	}
	c, err := minio.New(u.Host, &minio.Options{Creds: creds, Secure: u.Scheme == "https", Region: o.Region, BucketLookup: minio.BucketLookupPath})
	if err != nil {
		return nil, err
	}
	return &S3Store{client: c, bucket: o.Bucket}, nil
}
func (s *S3Store) Put(ctx context.Context, key string, data []byte, contentType string) error {
	if err := validKey(key); err != nil {
		return err
	}
	_, err := s.client.PutObject(ctx, s.bucket, key, bytes.NewReader(data), int64(len(data)), minio.PutObjectOptions{ContentType: contentType})
	return err
}
func (s *S3Store) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	if err := validKey(key); err != nil {
		return nil, err
	}
	obj, err := s.client.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, err
	}
	// GetObject is lazy: surface missing objects before sending HTTP 200.
	if _, err = obj.Stat(); err != nil {
		obj.Close()
		return nil, err
	}
	return obj, nil
}
func (s *S3Store) Delete(ctx context.Context, key string) error {
	if err := validKey(key); err != nil {
		return err
	}
	return s.client.RemoveObject(ctx, s.bucket, key, minio.RemoveObjectOptions{})
}
func (s *S3Store) SignURL(ctx context.Context, key string, ttl time.Duration) (string, error) {
	if err := validKey(key); err != nil {
		return "", err
	}
	if ttl < time.Second || ttl > 7*24*time.Hour {
		return "", fmt.Errorf("signed URL TTL must be between one second and seven days")
	}
	u, err := s.client.PresignedGetObject(ctx, s.bucket, key, ttl, nil)
	if err != nil {
		return "", err
	}
	return u.String(), nil
}
