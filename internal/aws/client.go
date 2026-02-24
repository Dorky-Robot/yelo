package aws

import (
	"context"
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

type ObjectInfo struct {
	Key           string
	Size          int64
	LastModified  string
	StorageClass  string
	RestoreStatus string // "", "in-progress", "available", "expired"
	ContentType   string
	ETag          string
	IsPrefix      bool
}

type RestoreInput struct {
	Bucket string
	Key    string
	Days   int32
	Tier   types.Tier
}

type ProgressFunc func(bytesTransferred int64, totalBytes int64)

type S3Client interface {
	ListBuckets(ctx context.Context) ([]string, error)
	ListObjects(ctx context.Context, bucket, prefix string, recursive bool) ([]ObjectInfo, error)
	HeadObject(ctx context.Context, bucket, key string) (*ObjectInfo, error)
	RestoreObject(ctx context.Context, input RestoreInput) error
	Download(ctx context.Context, bucket, key string, w io.Writer, progress ProgressFunc) error
	Upload(ctx context.Context, bucket, key string, r io.Reader, size int64, storageClass string, progress ProgressFunc) error
}

type Client struct {
	svc *s3.Client
}

func NewClient(ctx context.Context, region, profile string) (*Client, error) {
	var opts []func(*awsconfig.LoadOptions) error

	if region != "" {
		opts = append(opts, awsconfig.WithRegion(region))
	}
	if profile != "" {
		opts = append(opts, awsconfig.WithSharedConfigProfile(profile))
	}

	cfg, err := awsconfig.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("loading AWS config: %w", err)
	}

	return &Client{svc: s3.NewFromConfig(cfg)}, nil
}

func (c *Client) ListBuckets(ctx context.Context) ([]string, error) {
	out, err := c.svc.ListBuckets(ctx, &s3.ListBucketsInput{})
	if err != nil {
		return nil, fmt.Errorf("listing buckets: %w", err)
	}

	names := make([]string, len(out.Buckets))
	for i, b := range out.Buckets {
		names[i] = aws.ToString(b.Name)
	}
	return names, nil
}

func (c *Client) ListObjects(ctx context.Context, bucket, prefix string, recursive bool) ([]ObjectInfo, error) {
	var objects []ObjectInfo

	input := &s3.ListObjectsV2Input{
		Bucket: aws.String(bucket),
	}
	if prefix != "" {
		input.Prefix = aws.String(prefix)
	}
	if !recursive {
		input.Delimiter = aws.String("/")
	}

	paginator := s3.NewListObjectsV2Paginator(c.svc, input)
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("listing objects: %w", err)
		}

		for _, p := range page.CommonPrefixes {
			objects = append(objects, ObjectInfo{
				Key:      aws.ToString(p.Prefix),
				IsPrefix: true,
			})
		}

		for _, obj := range page.Contents {
			info := ObjectInfo{
				Key:          aws.ToString(obj.Key),
				Size:         aws.ToInt64(obj.Size),
				StorageClass: string(obj.StorageClass),
			}
			if obj.LastModified != nil {
				info.LastModified = obj.LastModified.Format("2006-01-02T15:04:05Z")
			}
			if info.StorageClass == "" {
				info.StorageClass = "STANDARD"
			}
			objects = append(objects, info)
		}
	}

	return objects, nil
}

func (c *Client) HeadObject(ctx context.Context, bucket, key string) (*ObjectInfo, error) {
	out, err := c.svc.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, fmt.Errorf("head object: %w", err)
	}

	info := &ObjectInfo{
		Key:          key,
		Size:         aws.ToInt64(out.ContentLength),
		StorageClass: string(out.StorageClass),
		ContentType:  aws.ToString(out.ContentType),
		ETag:         aws.ToString(out.ETag),
	}
	if out.LastModified != nil {
		info.LastModified = out.LastModified.Format("2006-01-02T15:04:05Z")
	}
	if info.StorageClass == "" {
		info.StorageClass = "STANDARD"
	}

	if out.Restore != nil {
		info.RestoreStatus = ParseRestoreHeader(aws.ToString(out.Restore))
	}

	return info, nil
}
