// Package version provides a non-blocking check for newer CLI releases on GitHub.
package version

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	repo           = "wherobots/wbc-cli"
	checkTimeout   = 3 * time.Second
	collectTimeout = 5 * time.Second
)

// Result holds the outcome of an update check.
type Result struct {
	Current  string // version the running binary was built with
	Latest   string // latest release tag from GitHub
	Outdated bool   // true when current < latest
}

// CheckInBackground spawns a goroutine that queries the GitHub API for the
// latest release tag. Call Collect on the returned channel after the main
// command has finished to retrieve the result (if any).
//
// The check is skipped entirely when currentVersion is a development value
// (e.g. "dev" or "latest-prerelease") because there is nothing meaningful to
// compare against.
func CheckInBackground(ctx context.Context, currentVersion string) <-chan *Result {
	ch := make(chan *Result, 1)

	if isDevVersion(currentVersion) {
		close(ch)
		return ch
	}

	go func() {
		defer close(ch)

		checkCtx, cancel := context.WithTimeout(ctx, checkTimeout)
		defer cancel()

		latest, err := fetchLatestTag(checkCtx)
		if err != nil || latest == "" {
			return // silently skip; don't annoy users when the check fails
		}

		if !isNewer(currentVersion, latest) {
			return
		}

		ch <- &Result{
			Current:  currentVersion,
			Latest:   latest,
			Outdated: true,
		}
	}()

	return ch
}

// Collect waits briefly for the background check result. Returns nil if no
// update is available, the check was skipped, or the timeout elapsed.
func Collect(ch <-chan *Result) *Result {
	select {
	case r := <-ch:
		return r
	case <-time.After(collectTimeout):
		return nil
	}
}

// FormatNotice returns the human-readable update message to display.
func FormatNotice(r *Result) string {
	return fmt.Sprintf(
		"A newer version of the Wherobots CLI is available: %s (current: %s).\nRun `wherobots upgrade` to update.",
		r.Latest, r.Current,
	)
}

// -------------------------------------------------------------------
// internal helpers
// -------------------------------------------------------------------

func isDevVersion(v string) bool {
	v = strings.TrimSpace(strings.ToLower(v))
	return v == "" || v == "dev" || v == "latest-prerelease" || strings.HasPrefix(v, "dev-")
}

// fetchLatestTag queries the GitHub API for the latest release tag.
func fetchLatestTag(ctx context.Context) (string, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "wherobots-cli")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	var release struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", err
	}
	return strings.TrimSpace(release.TagName), nil
}

// isNewer returns true when latest represents a strictly newer semver than current.
func isNewer(current, latest string) bool {
	cv := parseSemver(current)
	lv := parseSemver(latest)
	if cv == nil || lv == nil {
		// Fall back to plain string comparison when versions are not semver.
		return normTag(latest) != normTag(current)
	}
	return compareSemver(lv, cv) > 0
}

type semver struct {
	Major, Minor, Patch int
}

func parseSemver(s string) *semver {
	s = normTag(s)
	parts := strings.SplitN(s, ".", 3)
	if len(parts) != 3 {
		return nil
	}
	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return nil
	}
	minor, err := strconv.Atoi(parts[1])
	if err != nil {
		return nil
	}
	// Patch may carry a pre-release suffix like "0-rc1"; strip it.
	patchStr := parts[2]
	if idx := strings.IndexAny(patchStr, "-+"); idx >= 0 {
		patchStr = patchStr[:idx]
	}
	patch, err := strconv.Atoi(patchStr)
	if err != nil {
		return nil
	}
	return &semver{Major: major, Minor: minor, Patch: patch}
}

func compareSemver(a, b *semver) int {
	if a.Major != b.Major {
		return a.Major - b.Major
	}
	if a.Minor != b.Minor {
		return a.Minor - b.Minor
	}
	return a.Patch - b.Patch
}

func normTag(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "v")
	return s
}
