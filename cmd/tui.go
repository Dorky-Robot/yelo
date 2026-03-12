package cmd

import (
	"context"

	tuiapp "github.com/dorkyrobot/yelo/internal/tui/app"
)

func init() {
	register("tui", runTUI, "tui — interactive terminal UI")
}

func runTUI(env *Env, args []string) error {
	bucket, err := env.ResolveBucket()
	if err != nil {
		return err
	}

	ctx := context.Background()
	client, err := env.NewS3Client(ctx)
	if err != nil {
		return err
	}

	return tuiapp.Run(env.Cfg, env.State, client, bucket)
}
