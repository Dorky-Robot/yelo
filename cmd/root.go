package cmd

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/dorkyrobot/yelo/internal/aws"
	"github.com/dorkyrobot/yelo/internal/config"
	"github.com/dorkyrobot/yelo/internal/state"
)

type GlobalOpts struct {
	Bucket  string
	Region  string
	Profile string
	Config  string
}

// Env holds resolved runtime dependencies for subcommands.
type Env struct {
	Cfg   *config.Config
	State *state.State
	Opts  GlobalOpts
}

// NewS3Client creates an S3 client using resolved region/profile from config.
func (e *Env) NewS3Client(ctx context.Context) (aws.S3Client, error) {
	bucket, err := e.ResolveBucket()
	if err != nil {
		return nil, err
	}
	region := e.Cfg.ResolveRegion(e.Opts.Region, bucket)
	profile := e.Cfg.ResolveProfile(e.Opts.Profile, bucket)
	return aws.NewClient(ctx, region, profile)
}

// ResolveBucket returns the effective bucket from flags, state, or config.
func (e *Env) ResolveBucket() (string, error) {
	if e.Opts.Bucket != "" {
		return e.Opts.Bucket, nil
	}
	if e.State.Bucket != "" {
		return e.State.Bucket, nil
	}
	return e.Cfg.ResolveBucket("")
}

type subcommand struct {
	run   func(env *Env, args []string) error
	usage string
}

var commands = map[string]subcommand{}

func register(name string, run func(env *Env, args []string) error, usage string) {
	commands[name] = subcommand{run: run, usage: usage}
}

func Execute() error {
	var opts GlobalOpts
	flag.StringVar(&opts.Bucket, "bucket", "", "S3 bucket name")
	flag.StringVar(&opts.Region, "region", "", "AWS region")
	flag.StringVar(&opts.Profile, "profile", "", "AWS profile")
	flag.StringVar(&opts.Config, "config", "", "config file path")

	flag.Usage = func() { printUsage() }
	flag.Parse()

	args := flag.Args()
	if len(args) == 0 {
		printUsage()
		return nil
	}

	name := args[0]
	rest := args[1:]

	if name == "help" {
		printUsage()
		return nil
	}

	sub, ok := commands[name]
	if !ok {
		fmt.Fprintf(os.Stderr, "yelo: unknown command %q\n\n", name)
		printUsage()
		return fmt.Errorf("unknown command %q", name)
	}

	cfg, err := config.Load(opts.Config)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	st, err := state.Load("")
	if err != nil {
		return fmt.Errorf("loading state: %w", err)
	}

	env := &Env{
		Cfg:   cfg,
		State: st,
		Opts:  opts,
	}

	return sub.run(env, rest)
}

func printUsage() {
	fmt.Fprint(os.Stderr, `yelo — S3/Glacier CLI with FTP-style navigation

Usage: yelo [flags] <command> [args]

Commands:
  ls [path]                    List objects and prefixes
  get <remote> [local]         Download an object
  put <local> [remote]         Upload an object
  restore <path>               Initiate Glacier restore
  stat <path>                  Show object metadata
  buckets [list|add|remove|default]  Manage configured buckets
  cd <path>                    Set working directory
  pwd                          Show current bucket and prefix

Global flags:
  --bucket   S3 bucket name
  --region   AWS region
  --profile  AWS profile
  --config   Config file path

`)
}
