package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/dorkyrobot/yelo/internal/state"
	"github.com/dorkyrobot/yelo/internal/tui"
)

func init() {
	register("get", runGet, "get <remote> [local] — download an object")
}

func runGet(env *Env, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: yelo get <remote> [local]")
	}

	bucket, err := env.ResolveBucket()
	if err != nil {
		return err
	}

	key := state.ResolvePath(env.State.Prefix, args[0])
	if key == "" {
		return fmt.Errorf("get requires an object key, not a directory")
	}

	// Determine local destination.
	var localPath string
	toStdout := false
	if len(args) >= 2 {
		if args[1] == "-" {
			toStdout = true
		} else {
			localPath = args[1]
		}
	} else {
		localPath = filepath.Base(key)
	}

	ctx := context.Background()
	client, err := env.NewS3Client(ctx)
	if err != nil {
		return err
	}

	// Check Glacier status before attempting download.
	info, err := client.HeadObject(ctx, bucket, key)
	if err != nil {
		return err
	}

	if isGlacierClass(info.StorageClass) && info.RestoreStatus != "available" {
		if info.RestoreStatus == "in-progress" {
			return fmt.Errorf("object %q is being restored from %s (in progress); try again later", key, info.StorageClass)
		}
		return fmt.Errorf("object %q is in %s storage; restore it first with: yelo restore %s", key, info.StorageClass, args[0])
	}

	if toStdout {
		progress := tui.NewProgressFunc(key)
		return client.Download(ctx, bucket, key, os.Stdout, progress)
	}

	f, err := os.Create(localPath)
	if err != nil {
		return fmt.Errorf("creating %s: %w", localPath, err)
	}
	defer f.Close()

	progress := tui.NewProgressFunc(key)
	if err := client.Download(ctx, bucket, key, f, progress); err != nil {
		os.Remove(localPath)
		return err
	}

	fmt.Fprintf(os.Stderr, "%s → %s\n", key, localPath)
	return nil
}
