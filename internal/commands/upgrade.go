package commands

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
)

const (
	upgradeRepo       = "wherobots/wbc-cli"
	upgradeDefaultTag = "latest-prerelease"
	upgradeBinary     = "wherobots"
)

type upgradeOptions struct {
	tag            string
	skipChecksum   bool
	installDir     string
	currentVersion string
}

// AddUpgradeCommand registers the "upgrade" subcommand on the root command.
// It requires the current build version to display during the upgrade flow.
func AddUpgradeCommand(root *cobra.Command, currentVersion string) {
	opts := &upgradeOptions{currentVersion: currentVersion}

	cmd := &cobra.Command{
		Use:           "upgrade",
		Short:         "Upgrade the CLI to the latest release",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runUpgrade(cmd, opts)
		},
	}

	cmd.Flags().StringVar(&opts.tag, "tag", upgradeDefaultTag, "release tag to install")
	cmd.Flags().BoolVar(&opts.skipChecksum, "skip-checksum", false, "skip SHA-256 checksum verification")
	cmd.Flags().StringVar(&opts.installDir, "install-dir", "", "override install directory (default: directory of the current binary)")

	root.AddCommand(cmd)
}

func runUpgrade(cmd *cobra.Command, opts *upgradeOptions) error {
	w := cmd.ErrOrStderr()

	// Resolve the install directory: prefer flag, then location of the running binary.
	installDir := opts.installDir
	if installDir == "" {
		exe, err := os.Executable()
		if err != nil {
			return fmt.Errorf("cannot determine current binary location: %w", err)
		}
		exe, err = filepath.EvalSymlinks(exe)
		if err != nil {
			return fmt.Errorf("cannot resolve binary symlink: %w", err)
		}
		installDir = filepath.Dir(exe)
	}

	// Ensure gh is available and authenticated.
	if err := requireGh(); err != nil {
		return err
	}

	osName, archName, err := detectPlatform()
	if err != nil {
		return err
	}

	asset := fmt.Sprintf("%s_%s_%s", upgradeBinary, osName, archName)
	tag := opts.tag

	fmt.Fprintf(w, "Downloading %s from %s@%s...\n", asset, upgradeRepo, tag)

	tmpDir, err := os.MkdirTemp("", "wherobots-upgrade-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	if err := ghDownload(tag, asset, tmpDir); err != nil {
		return fmt.Errorf("download asset: %w", err)
	}

	assetPath := filepath.Join(tmpDir, asset)

	if !opts.skipChecksum {
		fmt.Fprintln(w, "Verifying checksum...")
		if err := ghDownload(tag, "checksums.txt", tmpDir); err != nil {
			return fmt.Errorf("download checksums: %w", err)
		}
		if err := verifyChecksum(assetPath, filepath.Join(tmpDir, "checksums.txt"), asset); err != nil {
			return err
		}
	}

	target := filepath.Join(installDir, upgradeBinary)

	if err := installBinary(assetPath, target); err != nil {
		return err
	}

	fmt.Fprintf(w, "Installed: %s\n", target)
	return nil
}

// requireGh checks that the gh CLI is installed and authenticated.
func requireGh() error {
	if _, err := exec.LookPath("gh"); err != nil {
		return fmt.Errorf("gh CLI is required; install from https://cli.github.com/")
	}
	if err := exec.Command("gh", "auth", "status").Run(); err != nil {
		return fmt.Errorf("gh is not authenticated; run: gh auth login")
	}
	return nil
}

func detectPlatform() (string, string, error) {
	var osName string
	switch runtime.GOOS {
	case "darwin":
		osName = "darwin"
	case "linux":
		osName = "linux"
	case "windows":
		osName = "windows"
	default:
		return "", "", fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}

	var archName string
	switch runtime.GOARCH {
	case "amd64":
		archName = "amd64"
	case "arm64":
		archName = "arm64"
	default:
		return "", "", fmt.Errorf("unsupported architecture: %s", runtime.GOARCH)
	}

	return osName, archName, nil
}

func ghDownload(tag, pattern, dir string) error {
	out, err := exec.Command("gh", "release", "download", tag,
		"--repo", upgradeRepo,
		"--pattern", pattern,
		"--dir", dir,
		"--clobber",
	).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func verifyChecksum(assetPath, checksumsPath, assetName string) error {
	data, err := os.ReadFile(checksumsPath)
	if err != nil {
		return fmt.Errorf("read checksums file: %w", err)
	}

	var expected string
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[1] == assetName {
			expected = fields[0]
			break
		}
	}
	if expected == "" {
		return fmt.Errorf("no checksum entry found for %s", assetName)
	}

	f, err := os.Open(assetPath)
	if err != nil {
		return fmt.Errorf("open asset for checksum: %w", err)
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return fmt.Errorf("hash asset: %w", err)
	}
	actual := hex.EncodeToString(h.Sum(nil))

	if actual != expected {
		return fmt.Errorf("checksum mismatch for %s:\n  expected: %s\n  actual:   %s", assetName, expected, actual)
	}
	return nil
}

func installBinary(src, dst string) error {
	// Read the downloaded binary into memory so we can write it even if the
	// target is the currently running executable (write-to-temp + rename).
	data, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("read downloaded binary: %w", err)
	}

	dir := filepath.Dir(dst)

	// Try writing directly first.
	tmp, err := os.CreateTemp(dir, ".wherobots-upgrade-*")
	if err != nil {
		// Might lack write permission; try with sudo via install(1).
		return installWithSudo(src, dst)
	}
	tmpPath := tmp.Name()

	if _, writeErr := tmp.Write(data); writeErr != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("write binary: %w", writeErr)
	}
	if err := tmp.Chmod(0755); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("chmod binary: %w", err)
	}
	tmp.Close()

	if err := os.Rename(tmpPath, dst); err != nil {
		os.Remove(tmpPath)
		return installWithSudo(src, dst)
	}
	return nil
}

func installWithSudo(src, dst string) error {
	if _, err := exec.LookPath("sudo"); err != nil {
		return fmt.Errorf("no write access to %s and sudo is unavailable", filepath.Dir(dst))
	}
	dir := filepath.Dir(dst)
	if err := exec.Command("sudo", "mkdir", "-p", dir).Run(); err != nil {
		return fmt.Errorf("sudo mkdir: %w", err)
	}
	if err := exec.Command("sudo", "install", "-m", "0755", src, dst).Run(); err != nil {
		return fmt.Errorf("sudo install: %w", err)
	}
	return nil
}
