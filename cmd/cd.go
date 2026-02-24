package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/dorkyrobot/yelo/internal/state"
)

func init() {
	register("cd", runCD, "cd <path> — set working directory")
}

func runCD(env *Env, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: yelo cd <path>")
	}

	target := args[0]

	// Allow "cd bucket:path" to switch bucket and path at once.
	if bucket, p := parseBucketPath(target); bucket != "" {
		env.State.SetBucket(bucket)
		target = p
		if target == "" {
			target = "/"
		}
	}

	prefix := state.ResolvePath(env.State.Prefix, target)
	if prefix != "" && !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}

	env.State.Prefix = prefix
	if err := env.State.Save(); err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "/%s\n", prefix)
	return nil
}
