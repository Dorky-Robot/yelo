package cmd

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/dorkyrobot/yelo/internal/state"
	"github.com/dorkyrobot/yelo/internal/tui"
)

func init() {
	register("put", runPut, "put [flags] <local> [remote] — upload an object")
}

func runPut(env *Env, args []string) error {
	fs := flag.NewFlagSet("put", flag.ContinueOnError)
	storageClass := fs.String("storage-class", "DEEP_ARCHIVE", "S3 storage class")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if fs.NArg() == 0 {
		return fmt.Errorf("usage: yelo put [--storage-class CLASS] <local> [remote]")
	}

	bucket, err := env.ResolveBucket()
	if err != nil {
		return err
	}

	localPath := fs.Arg(0)

	// Determine the remote key.
	var key string
	if fs.NArg() >= 2 {
		key = state.ResolvePath(env.State.Prefix, fs.Arg(1))
	} else {
		// Default: current prefix + local filename.
		name := filepath.Base(localPath)
		key = state.ResolvePath(env.State.Prefix, name)
	}

	f, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("opening %s: %w", localPath, err)
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return fmt.Errorf("stat %s: %w", localPath, err)
	}
	size := info.Size()

	ctx := context.Background()
	client, err := env.NewS3Client(ctx)
	if err != nil {
		return err
	}

	progress := tui.NewProgressFunc(filepath.Base(localPath))
	if err := client.Upload(ctx, bucket, key, f, size, *storageClass, progress); err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "%s → s3://%s/%s (%s)\n", localPath, bucket, key, *storageClass)
	return nil
}
