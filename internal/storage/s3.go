package storage

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/CookSleep/lillian_backend/internal/config"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type ObjectStore interface {
	PutBytes(ctx context.Context, key string, data []byte, contentType string) (string, error)
	Get(ctx context.Context, key string) (io.ReadCloser, string, error)
	Delete(ctx context.Context, key string) error
	PublicURL(key string) string
}

type S3Store struct {
	cfg    config.StorageConfig
	client *s3.Client
}

func NewS3Store(ctx context.Context, cfg config.StorageConfig) (*S3Store, error) {
	if cfg.Bucket == "" {
		return nil, nil
	}
	if cfg.Region == "" {
		cfg.Region = "auto"
	}

	options := []func(*awsconfig.LoadOptions) error{
		awsconfig.WithRegion(cfg.Region),
	}
	if cfg.AccessKeyID != "" || cfg.SecretAccessKey != "" {
		options = append(options, awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(cfg.AccessKeyID, cfg.SecretAccessKey, ""),
		))
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, options...)
	if err != nil {
		return nil, err
	}

	client := s3.NewFromConfig(awsCfg, func(options *s3.Options) {
		if cfg.Endpoint != "" {
			options.BaseEndpoint = aws.String(cfg.Endpoint)
		}
		options.UsePathStyle = cfg.ForcePathStyle
	})

	return &S3Store{cfg: cfg, client: client}, nil
}

func (s *S3Store) PutBytes(ctx context.Context, key string, data []byte, contentType string) (string, error) {
	if s == nil {
		return "", fmt.Errorf("object store is not configured")
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.cfg.Bucket),
		Key:         aws.String(cleanKey(key)),
		Body:        bytes.NewReader(data),
		ContentType: aws.String(contentType),
	})
	if err != nil {
		return "", err
	}
	return s.PublicURL(key), nil
}

func (s *S3Store) Get(ctx context.Context, key string) (io.ReadCloser, string, error) {
	if s == nil {
		return nil, "", fmt.Errorf("object store is not configured")
	}
	resp, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.cfg.Bucket),
		Key:    aws.String(cleanKey(key)),
	})
	if err != nil {
		return nil, "", err
	}
	contentType := ""
	if resp.ContentType != nil {
		contentType = *resp.ContentType
	}
	return resp.Body, contentType, nil
}

func (s *S3Store) Delete(ctx context.Context, key string) error {
	if s == nil {
		return fmt.Errorf("object store is not configured")
	}
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.cfg.Bucket),
		Key:    aws.String(cleanKey(key)),
	})
	return err
}

func (s *S3Store) PublicURL(key string) string {
	if s == nil {
		return ""
	}
	clean := publicPath(key)
	if s.cfg.PublicBaseURL != "" {
		return s.cfg.PublicBaseURL + "/" + clean
	}
	if s.cfg.Endpoint != "" {
		return strings.TrimRight(s.cfg.Endpoint, "/") + "/" + s.cfg.Bucket + "/" + clean
	}
	return ""
}

func cleanKey(key string) string {
	return strings.TrimLeft(strings.TrimSpace(key), "/")
}

func publicPath(key string) string {
	segments := strings.Split(cleanKey(key), "/")
	for i := range segments {
		segments[i] = url.PathEscape(segments[i])
	}
	return strings.Join(segments, "/")
}
