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

	var loadOpts []config.Option
	if hasFlag(os.Args[1:], "staging") {
		loadOpts = append(loadOpts, config.WithStaging())
	}

	cfg, err := config.Load(loadOpts...)
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

// hasFlag reports whether the given boolean flag is set in args.
// It recognises --name, --name=true, and --name=false.
func hasFlag(args []string, name string) bool {
	for _, arg := range args {
		if arg == "--"+name || arg == "--"+name+"=true" {
			return true
		}
		if arg == "--"+name+"=false" {
			return false
		}
		// Stop scanning at "--" (end-of-flags sentinel).
		if arg == "--" {
			return false
		}
	}
	return false
}
