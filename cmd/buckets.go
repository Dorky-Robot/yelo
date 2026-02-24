package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/dorkyrobot/yelo/internal/aws"
)

func init() {
	register("buckets", runBuckets, "buckets [list|add|remove|default] — manage configured buckets")
}

func runBuckets(env *Env, args []string) error {
	sub := "list"
	if len(args) > 0 {
		sub = args[0]
		args = args[1:]
	}

	switch sub {
	case "list":
		return bucketsListCmd(env, args)
	case "add":
		return bucketsAddCmd(env, args)
	case "remove":
		return bucketsRemoveCmd(env, args)
	case "default":
		return bucketsDefaultCmd(env, args)
	default:
		return fmt.Errorf("unknown buckets subcommand %q; use list, add, remove, or default", sub)
	}
}

func bucketsListCmd(env *Env, args []string) error {
	remote := len(args) > 0 && args[0] == "--remote"

	if remote || len(env.Cfg.Buckets) == 0 {
		ctx := context.Background()
		region := env.Cfg.ResolveRegion(env.Opts.Region, "")
		profile := env.Cfg.ResolveProfile(env.Opts.Profile, "")

		client, err := aws.NewClient(ctx, region, profile)
		if err != nil {
			return err
		}

		buckets, err := client.ListBuckets(ctx)
		if err != nil {
			return err
		}

		for _, b := range buckets {
			fmt.Println(b)
		}
		return nil
	}

	for _, b := range env.Cfg.Buckets {
		marker := "  "
		if b.Name == env.Cfg.DefaultBucket {
			marker = "* "
		}
		line := marker + b.Name
		if b.Region != "" {
			line += fmt.Sprintf(" (region=%s)", b.Region)
		}
		if b.Profile != "" {
			line += fmt.Sprintf(" (profile=%s)", b.Profile)
		}
		fmt.Println(line)
	}
	return nil
}

func bucketsAddCmd(env *Env, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: yelo buckets add <name> [--region REGION] [--profile PROFILE]")
	}

	name := args[0]
	var region, profile string
	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--region":
			if i+1 < len(args) {
				i++
				region = args[i]
			}
		case "--profile":
			if i+1 < len(args) {
				i++
				profile = args[i]
			}
		}
	}

	env.Cfg.AddBucket(name, region, profile)
	if err := env.Cfg.Save(); err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "added bucket %s\n", name)
	return nil
}

func bucketsRemoveCmd(env *Env, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: yelo buckets remove <name>")
	}

	name := args[0]
	if !env.Cfg.RemoveBucket(name) {
		return fmt.Errorf("bucket %q not found in config", name)
	}

	if err := env.Cfg.Save(); err != nil {
		return err
	}

	if env.State.Bucket == name {
		env.State.SetBucket("")
		if err := env.State.Save(); err != nil {
			return err
		}
	}

	fmt.Fprintf(os.Stderr, "removed bucket %s\n", name)
	return nil
}

func bucketsDefaultCmd(env *Env, args []string) error {
	if len(args) == 0 {
		if env.Cfg.DefaultBucket == "" {
			fmt.Println("(no default bucket set)")
		} else {
			fmt.Println(env.Cfg.DefaultBucket)
		}
		return nil
	}

	name := args[0]
	if err := env.Cfg.SetDefault(name); err != nil {
		return err
	}

	if err := env.Cfg.Save(); err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "default bucket set to %s\n", name)
	return nil
}
