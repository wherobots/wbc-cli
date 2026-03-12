package main

import (
	"context"
	"fmt"
	"os"

	"wherobots/cli/internal/commands"
	"wherobots/cli/internal/config"
	"wherobots/cli/internal/spec"
	"wherobots/cli/internal/version"
)

var (
	buildVersion = "dev"
	commit       = "none"
	date         = "unknown"
)

func main() {
	if err := run(context.Background()); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	// Start a background update check early so it runs in parallel with setup.
	updateCh := version.CheckInBackground(ctx, buildVersion)

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

	versionString := fmt.Sprintf("%s (commit %s, built %s)", buildVersion, commit, date)

	root := commands.BuildRootCommand(cfg, runtimeSpec)
	root.Version = versionString
	root.Short = fmt.Sprintf("Wherobots CLI %s", buildVersion)
	commands.AddUpgradeCommand(root, buildVersion)
	commands.AddVersionCommand(root, versionString)
	execErr := root.ExecuteContext(ctx)

	// After the command finishes, print an update notice if one is available.
	if result := version.Collect(updateCh); result != nil {
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, version.FormatNotice(result))
	}

	return execErr
}
