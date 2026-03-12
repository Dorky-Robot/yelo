package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/dorkyrobot/yelo/internal/aws"
)

func fetchList(client aws.S3Client, bucket, prefix string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		items, err := client.ListObjects(ctx, bucket, prefix, false)
		return listResultMsg{items: items, err: err}
	}
}

func fetchDetail(client aws.S3Client, bucket, key string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		info, err := client.HeadObject(ctx, bucket, key)
		return detailResultMsg{info: info, err: err}
	}
}

func fetchBuckets(client aws.S3Client) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		buckets, err := client.ListBuckets(ctx)
		return bucketsResultMsg{buckets: buckets, err: err}
	}
}

func downloadObject(client aws.S3Client, bucket, key string) tea.Cmd {
	return func() tea.Msg {
		localPath := filepath.Base(key)
		f, err := os.Create(localPath)
		if err != nil {
			return downloadCompleteMsg{key: key, err: fmt.Errorf("creating %s: %w", localPath, err)}
		}
		defer f.Close()

		ctx := context.Background()
		if err := client.Download(ctx, bucket, key, f, nil); err != nil {
			os.Remove(localPath)
			return downloadCompleteMsg{key: key, err: err}
		}
		return downloadCompleteMsg{key: key, localPath: localPath}
	}
}

func restoreObject(client aws.S3Client, bucket, key string, days int32, tier string) tea.Cmd {
	return func() tea.Msg {
		parsedTier, err := aws.ParseTier(tier)
		if err != nil {
			return restoreCompleteMsg{key: key, err: err}
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		err = client.RestoreObject(ctx, aws.RestoreInput{
			Bucket: bucket,
			Key:    key,
			Days:   days,
			Tier:   parsedTier,
		})
		return restoreCompleteMsg{key: key, err: err}
	}
}

func clearFlashAfter(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(time.Time) tea.Msg {
		return clearFlashMsg{}
	})
}
