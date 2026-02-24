package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/dorkyrobot/yelo/internal/output"
	"github.com/dorkyrobot/yelo/internal/state"
)

func init() {
	register("stat", runStat, "stat <path> — show object metadata")
}

func runStat(env *Env, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: yelo stat <path>")
	}

	bucket, err := env.ResolveBucket()
	if err != nil {
		return err
	}

	key := state.ResolvePath(env.State.Prefix, args[0])
	if key == "" {
		return fmt.Errorf("stat requires an object key, not a directory")
	}

	ctx := context.Background()
	client, err := env.NewS3Client(ctx)
	if err != nil {
		return err
	}

	obj, err := client.HeadObject(ctx, bucket, key)
	if err != nil {
		return err
	}

	tty := output.IsTTY(os.Stdout)
	output.FormatStat(os.Stdout, obj, tty)
	return nil
}
