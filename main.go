package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

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

// startNoticeWait is how long we wait for the background update check to
// finish before executing the user's command. Spec loading typically gives
// the check enough time to return; if not, we fall back to printing at the
// end so we never block the command on a slow network probe.
const startNoticeWait = 500 * time.Millisecond

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

	// Suppress the "update available" notice when the user is already running
	// `upgrade` — the check races with the upgrade itself and would otherwise
	// nag about the very version that just got installed.
	suppressNotice := commands.IsUpgradeInvocation(root, os.Args[1:])

	noticeShown := false
	if !suppressNotice {
		if result := version.TryCollect(updateCh, startNoticeWait); result != nil {
			printUpdateNotice(os.Stderr, result)
			noticeShown = true
		}
	}

	execErr := root.ExecuteContext(ctx)

	// Fallback: if the check hadn't returned by the time we started executing,
	// print the notice at the end rather than dropping it.
	if !suppressNotice && !noticeShown {
		if result := version.Collect(updateCh); result != nil {
			fmt.Fprintln(os.Stderr, "")
			printUpdateNotice(os.Stderr, result)
			noticeShown = true
		}
	}

	if execErr != nil && noticeShown {
		fmt.Fprintln(os.Stderr, "Note: your CLI is out of date. Run `wherobots upgrade` to update — it may resolve this issue.")
	}

	return execErr
}

func printUpdateNotice(w io.Writer, r *version.Result) {
	notice := version.FormatNotice(r)
	if isTTY(w) {
		notice = "\033[1;33m" + notice + "\033[0m"
	}
	fmt.Fprintln(w, notice)
	fmt.Fprintln(w, "")
}

// isTTY reports whether w is an *os.File pointing at a terminal. Stdlib only;
// avoids pulling in a tty-detection dependency for a single notice.
func isTTY(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}
