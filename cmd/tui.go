package cmd

import (
	"context"

	"github.com/dorkyrobot/yelo/internal/aws"
	tuiapp "github.com/dorkyrobot/yelo/internal/tui/app"
)

func init() {
	register("tui", runTUI, "tui — interactive terminal UI")
}

func runTUI(env *Env, args []string) error {
	bucket, _ := env.ResolveBucket()

	// Build a client even without a bucket — use global region/profile.
	ctx := context.Background()
	region := env.Cfg.ResolveRegion(env.Opts.Region, bucket)
	profile := env.Cfg.ResolveProfile(env.Opts.Profile, bucket)
	client, err := aws.NewClient(ctx, region, profile)
	if err != nil {
		return err
	}

	return tuiapp.Run(env.Cfg, env.State, client, bucket)
}
