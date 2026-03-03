package main

import (
	"context"
	"fmt"
	"os"

	"wherobots/cli/internal/commands"
	"wherobots/cli/internal/config"
	"wherobots/cli/internal/spec"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	if err := run(context.Background()); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	loader := spec.NewLoader(cfg)
	rawSpec, err := loader.Load(ctx)
	if err != nil {
		return err
	}

	runtimeSpec, err := spec.Parse(rawSpec, cfg.OpenAPIURL)
	if err != nil {
		return err
	}

	root := commands.BuildRootCommand(cfg, runtimeSpec)
	root.Version = fmt.Sprintf("%s (commit %s, built %s)", version, commit, date)
	if err := root.ExecuteContext(ctx); err != nil {
		return err
	}
	return nil
}
