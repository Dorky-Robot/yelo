package cmd

import "fmt"

func init() {
	register("pwd", runPWD, "pwd — show current bucket and prefix")
}

func runPWD(env *Env, args []string) error {
	bucket := env.State.Bucket
	if bucket == "" {
		bucket, _ = env.Cfg.ResolveBucket(env.Opts.Bucket)
	}

	if bucket == "" {
		fmt.Println("(no bucket set)")
		return nil
	}

	fmt.Printf("%s:/%s\n", bucket, env.State.Prefix)
	return nil
}
