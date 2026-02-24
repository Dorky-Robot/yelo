package cmd

import (
	"context"
	"flag"
	"fmt"
	"os"

	internalaws "github.com/dorkyrobot/yelo/internal/aws"
	"github.com/dorkyrobot/yelo/internal/state"
)

func init() {
	register("restore", runRestore, "restore [flags] <path> — initiate Glacier restore")
}

func runRestore(env *Env, args []string) error {
	fs := flag.NewFlagSet("restore", flag.ContinueOnError)
	days := fs.Int("days", 7, "number of days to keep restored copy")
	tier := fs.String("tier", "Standard", "retrieval tier: Standard, Bulk, Expedited")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if fs.NArg() == 0 {
		return fmt.Errorf("usage: yelo restore [--days N] [--tier TIER] <path>")
	}

	bucket, err := env.ResolveBucket()
	if err != nil {
		return err
	}

	key := state.ResolvePath(env.State.Prefix, fs.Arg(0))
	if key == "" {
		return fmt.Errorf("restore requires an object key")
	}

	parsedTier, err := internalaws.ParseTier(*tier)
	if err != nil {
		return err
	}

	ctx := context.Background()
	client, err := env.NewS3Client(ctx)
	if err != nil {
		return err
	}

	// Check current status.
	info, err := client.HeadObject(ctx, bucket, key)
	if err != nil {
		return err
	}

	if !isGlacierClass(info.StorageClass) {
		return fmt.Errorf("object %q is in %s storage (not Glacier); restore not needed", key, info.StorageClass)
	}

	if err := internalaws.ValidateTier(info.StorageClass, parsedTier); err != nil {
		return err
	}

	switch info.RestoreStatus {
	case "in-progress":
		fmt.Fprintf(os.Stderr, "restore already in progress for %s\n", key)
		return nil
	case "available":
		fmt.Fprintf(os.Stderr, "object %s is already restored and available\n", key)
		return nil
	}

	err = client.RestoreObject(ctx, internalaws.RestoreInput{
		Bucket: bucket,
		Key:    key,
		Days:   int32(*days),
		Tier:   parsedTier,
	})
	if err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "restore initiated: %s (tier=%s, days=%d)\n", key, *tier, *days)
	return nil
}
