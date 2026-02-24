package cmd

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/dorkyrobot/yelo/internal/output"
	"github.com/dorkyrobot/yelo/internal/state"
)

func init() {
	register("ls", runLS, "ls [flags] [path] — list objects and prefixes")
}

func runLS(env *Env, args []string) error {
	fs := flag.NewFlagSet("ls", flag.ContinueOnError)
	long := fs.Bool("l", false, "long format")
	recursive := fs.Bool("R", false, "recursive listing")
	if err := fs.Parse(args); err != nil {
		return err
	}

	bucket, err := env.ResolveBucket()
	if err != nil {
		return err
	}

	prefix := env.State.Prefix
	if fs.NArg() > 0 {
		prefix = state.ResolvePrefix(env.State.Prefix, fs.Arg(0))
	}

	ctx := context.Background()
	client, err := env.NewS3Client(ctx)
	if err != nil {
		return err
	}

	objects, err := client.ListObjects(ctx, bucket, prefix, *recursive)
	if err != nil {
		return err
	}

	if len(objects) == 0 {
		fmt.Fprintln(os.Stderr, "no objects found")
		return nil
	}

	tty := output.IsTTY(os.Stdout)
	output.ListObjects(os.Stdout, objects, *long, tty)
	return nil
}
