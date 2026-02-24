package aws

import (
	"context"
	"fmt"
	"io"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

func (c *Client) Download(ctx context.Context, bucket, key string, w io.Writer, progress ProgressFunc) error {
	out, err := c.svc.GetObject(ctx, &s3.GetObjectInput{
		Bucket: awssdk.String(bucket),
		Key:    awssdk.String(key),
	})
	if err != nil {
		return fmt.Errorf("downloading %s: %w", key, err)
	}
	defer out.Body.Close()

	total := awssdk.ToInt64(out.ContentLength)
	var transferred int64

	buf := make([]byte, 32*1024)
	for {
		n, readErr := out.Body.Read(buf)
		if n > 0 {
			written, writeErr := w.Write(buf[:n])
			if writeErr != nil {
				return fmt.Errorf("writing download data: %w", writeErr)
			}
			transferred += int64(written)
			if progress != nil {
				progress(transferred, total)
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return fmt.Errorf("reading download stream: %w", readErr)
		}
	}

	return nil
}

func (c *Client) Upload(ctx context.Context, bucket, key string, r io.Reader, size int64, storageClass string, progress ProgressFunc) error {
	var sc types.StorageClass
	if storageClass != "" {
		sc = types.StorageClass(storageClass)
	}

	pr := &progressReader{r: r, total: size, progress: progress}

	input := &s3.PutObjectInput{
		Bucket: awssdk.String(bucket),
		Key:    awssdk.String(key),
		Body:   pr,
	}
	if sc != "" {
		input.StorageClass = sc
	}
	if size > 0 {
		input.ContentLength = awssdk.Int64(size)
	}

	_, err := c.svc.PutObject(ctx, input)
	if err != nil {
		return fmt.Errorf("uploading %s: %w", key, err)
	}
	return nil
}

type progressReader struct {
	r           io.Reader
	total       int64
	transferred int64
	progress    ProgressFunc
}

func (pr *progressReader) Read(p []byte) (int, error) {
	n, err := pr.r.Read(p)
	pr.transferred += int64(n)
	if pr.progress != nil && n > 0 {
		pr.progress(pr.transferred, pr.total)
	}
	return n, err
}
