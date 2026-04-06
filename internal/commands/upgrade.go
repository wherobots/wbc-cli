package commands

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
)

const (
	upgradeRepo       = "wherobots/wbc-cli"
	upgradeDefaultTag = "latest"
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

	osName, archName, err := detectPlatform()
	if err != nil {
		return err
	}

	asset := fmt.Sprintf("%s_%s_%s", upgradeBinary, osName, archName)
	tag := opts.tag

	// "latest" is not a real tag — resolve it to the actual latest release tag.
	if tag == "latest" {
		resolved, err := resolveLatestTag()
		if err != nil {
			return fmt.Errorf("resolve latest release: %w", err)
		}
		tag = resolved
	}

	fmt.Fprintf(w, "Downloading %s from %s@%s...\n", asset, upgradeRepo, tag)

	tmpDir, err := os.MkdirTemp("", "wherobots-upgrade-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	if err := httpDownload(tag, asset, tmpDir); err != nil {
		return fmt.Errorf("download asset: %w", err)
	}

	assetPath := filepath.Join(tmpDir, asset)

	if !opts.skipChecksum {
		fmt.Fprintln(w, "Verifying checksum...")
		if err := httpDownload(tag, "checksums.txt", tmpDir); err != nil {
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

// resolveLatestTag queries the GitHub API for the latest release tag.
func resolveLatestTag() (string, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", upgradeRepo)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "wherobots-cli")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("GitHub API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("no release found in %s (HTTP %d)", upgradeRepo, resp.StatusCode)
	}

	var release struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", fmt.Errorf("failed to parse GitHub API response: %w", err)
	}
	tag := strings.TrimSpace(release.TagName)
	if tag == "" {
		return "", fmt.Errorf("no release found in %s", upgradeRepo)
	}
	return tag, nil
}

// httpDownload downloads a release asset from GitHub to the given directory.
func httpDownload(tag, filename, dir string) error {
	url := fmt.Sprintf("https://github.com/%s/releases/download/%s/%s", upgradeRepo, tag, filename)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "wherobots-cli")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("download request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to download %s (HTTP %d)", filename, resp.StatusCode)
	}

	outPath := filepath.Join(dir, filename)
	f, err := os.Create(outPath)
	if err != nil {
		return fmt.Errorf("create file %s: %w", outPath, err)
	}
	defer f.Close()

	if _, err := io.Copy(f, resp.Body); err != nil {
		return fmt.Errorf("write file %s: %w", outPath, err)
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
