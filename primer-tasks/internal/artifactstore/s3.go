package artifactstore

import (
	"context"
	"errors"
	"io"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awscfg "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type S3Store struct {
	client    *s3.Client
	presigner *s3.PresignClient
	bucket    string
}

type S3Config struct {
	Endpoint, Region, Bucket, AccessKey, SecretKey, SessionToken string
	ForcePathStyle                                               bool
}

// NewS3 supports AWS S3 and S3-compatible MinIO. Credentials are supplied by
// the AWS credential chain unless explicit static values are configured.
func NewS3(ctx context.Context, cfg S3Config) (*S3Store, error) {
	if strings.TrimSpace(cfg.Bucket) == "" {
		return nil, errors.New("S3 bucket is required")
	}
	opts := []func(*awscfg.LoadOptions) error{awscfg.WithRegion(defaultString(cfg.Region, "us-east-1"))}
	if cfg.AccessKey != "" || cfg.SecretKey != "" {
		opts = append(opts, awscfg.WithCredentialsProvider(aws.NewCredentialsCache(staticProvider{access: cfg.AccessKey, secret: cfg.SecretKey, token: cfg.SessionToken})))
	}
	if cfg.Endpoint != "" {
		opts = append(opts, awscfg.WithBaseEndpoint(strings.TrimRight(cfg.Endpoint, "/")))
	}
	awsCfg, err := awscfg.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, err
	}
	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) { o.UsePathStyle = cfg.ForcePathStyle })
	return &S3Store{client: client, presigner: s3.NewPresignClient(client), bucket: cfg.Bucket}, nil
}
func defaultString(v, d string) string {
	if v == "" {
		return d
	}
	return v
}

type staticProvider struct{ access, secret, token string }

func (p staticProvider) Retrieve(context.Context) (aws.Credentials, error) {
	return aws.Credentials{AccessKeyID: p.access, SecretAccessKey: p.secret, SessionToken: p.token, CanExpire: false}, nil
}
func (s *S3Store) Put(ctx context.Context, key, contentType string, src io.Reader, size int64) (Object, error) {
	if err := ValidateKey(key); err != nil {
		return Object{}, err
	}
	out, err := s.client.PutObject(ctx, &s3.PutObjectInput{Bucket: &s.bucket, Key: &key, Body: src, ContentLength: aws.Int64(size), ContentType: aws.String(contentType)})
	if err != nil {
		return Object{}, err
	}
	etag := ""
	if out.ETag != nil {
		etag = *out.ETag
	}
	return Object{Key: key, ContentType: contentType, Size: size, ETag: etag}, nil
}
func (s *S3Store) Open(ctx context.Context, key string) (io.ReadCloser, Object, error) {
	if err := ValidateKey(key); err != nil {
		return nil, Object{}, err
	}
	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{Bucket: &s.bucket, Key: &key})
	if err != nil {
		return nil, Object{}, err
	}
	size := int64(0)
	if out.ContentLength != nil {
		size = *out.ContentLength
	}
	ct := ""
	if out.ContentType != nil {
		ct = *out.ContentType
	}
	return out.Body, Object{Key: key, ContentType: ct, Size: size}, nil
}
func (s *S3Store) Stat(ctx context.Context, key string) (Object, error) {
	if err := ValidateKey(key); err != nil {
		return Object{}, err
	}
	out, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{Bucket: &s.bucket, Key: &key})
	if err != nil {
		return Object{}, err
	}
	o := Object{Key: key}
	if out.ContentLength != nil {
		o.Size = *out.ContentLength
	}
	if out.ContentType != nil {
		o.ContentType = *out.ContentType
	}
	if out.ETag != nil {
		o.ETag = *out.ETag
	}
	return o, nil
}
func (s *S3Store) EnsureBucket(ctx context.Context) error {
	_, err := s.client.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: &s.bucket})
	if err != nil && !strings.Contains(strings.ToLower(err.Error()), "already exist") && !strings.Contains(strings.ToLower(err.Error()), "owned by you") {
		return err
	}
	return nil
}
func (s *S3Store) Delete(ctx context.Context, key string) error {
	if err := ValidateKey(key); err != nil {
		return err
	}
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: &s.bucket, Key: &key})
	return err
}
func (s *S3Store) Compose(ctx context.Context, key, contentType string, parts []string, size int64) (Object, error) {
	readers := make([]io.Reader, 0, len(parts))
	closers := make([]io.Closer, 0, len(parts))
	var total int64
	for _, part := range parts {
		f, obj, err := s.Open(ctx, part)
		if err != nil {
			for _, c := range closers {
				_ = c.Close()
			}
			return Object{}, err
		}
		readers = append(readers, f)
		closers = append(closers, f)
		total += obj.Size
	}
	obj, err := s.Put(ctx, key, contentType, io.MultiReader(readers...), total)
	for _, c := range closers {
		_ = c.Close()
	}
	if err != nil {
		return Object{}, err
	}
	if total != size {
		return Object{}, errors.New("part size mismatch")
	}
	return obj, nil
}
func (s *S3Store) PresignPut(ctx context.Context, key, contentType string, size int64, ttl time.Duration) (string, error) {
	if err := ValidateKey(key); err != nil {
		return "", err
	}
	out, err := s.presigner.PresignPutObject(ctx, &s3.PutObjectInput{Bucket: &s.bucket, Key: &key, ContentType: &contentType, ContentLength: &size}, s3.WithPresignExpires(ttl))
	if err != nil {
		return "", err
	}
	return out.URL, nil
}
