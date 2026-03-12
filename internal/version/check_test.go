package version

import (
	"testing"
)

func TestParseSemver(t *testing.T) {
	tests := []struct {
		input string
		want  *semver
	}{
		{"1.2.3", &semver{1, 2, 3}},
		{"v1.2.3", &semver{1, 2, 3}},
		{"0.0.1", &semver{0, 0, 1}},
		{"10.20.30", &semver{10, 20, 30}},
		{"1.2.3-rc1", &semver{1, 2, 3}},
		{"v1.2.3+build.42", &semver{1, 2, 3}},
		{"dev", nil},
		{"", nil},
		{"1.2", nil},
		{"abc.def.ghi", nil},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := parseSemver(tt.input)
			if tt.want == nil {
				if got != nil {
					t.Fatalf("parseSemver(%q) = %+v, want nil", tt.input, got)
				}
				return
			}
			if got == nil {
				t.Fatalf("parseSemver(%q) = nil, want %+v", tt.input, tt.want)
			}
			if *got != *tt.want {
				t.Fatalf("parseSemver(%q) = %+v, want %+v", tt.input, got, tt.want)
			}
		})
	}
}

func TestIsNewer(t *testing.T) {
	tests := []struct {
		current string
		latest  string
		want    bool
	}{
		{"1.0.0", "1.0.1", true},
		{"1.0.0", "1.1.0", true},
		{"1.0.0", "2.0.0", true},
		{"v1.0.0", "v1.0.1", true},
		{"1.0.1", "1.0.0", false},
		{"1.0.0", "1.0.0", false},
		{"v1.0.0", "v1.0.0", false},
		{"2.0.0", "1.9.9", false},
		{"0.1.0", "0.2.0", true},
	}

	for _, tt := range tests {
		t.Run(tt.current+"_vs_"+tt.latest, func(t *testing.T) {
			got := isNewer(tt.current, tt.latest)
			if got != tt.want {
				t.Fatalf("isNewer(%q, %q) = %v, want %v", tt.current, tt.latest, got, tt.want)
			}
		})
	}
}

func TestIsDevVersion(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"dev", true},
		{"", true},
		{"latest-prerelease", true},
		{"dev-abc123", true},
		{"1.0.0", false},
		{"v1.0.0", false},
		{"0.1.0", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := isDevVersion(tt.input)
			if got != tt.want {
				t.Fatalf("isDevVersion(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestFormatNotice(t *testing.T) {
	r := &Result{Current: "1.0.0", Latest: "1.1.0", Outdated: true}
	notice := FormatNotice(r)
	if notice == "" {
		t.Fatal("FormatNotice returned empty string")
	}
	if !contains(notice, "1.1.0") || !contains(notice, "1.0.0") {
		t.Fatalf("FormatNotice should mention both versions, got: %s", notice)
	}
	if !contains(notice, "wherobots upgrade") {
		t.Fatalf("FormatNotice should mention 'wherobots upgrade', got: %s", notice)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
