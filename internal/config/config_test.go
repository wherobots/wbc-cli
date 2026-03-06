package config

import "testing"

func TestLoadDefaultsToWherobotsOpenAPISpec(t *testing.T) {
	t.Setenv("WHEROBOTS_API_URL", "")
	t.Setenv("WHEROBOTS_API_KEY", "key-1")
	t.Setenv("WHEROBOTS_S3_BUCKET", "")
	t.Setenv("WHEROBOTS_S3_PREFIX", "")
	t.Setenv("WHEROBOTS_UPLOAD_PATH", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.AppName != "wherobots" {
		t.Fatalf("AppName = %q, want %q", cfg.AppName, "wherobots")
	}
	if cfg.OpenAPIURL != "https://api.cloud.wherobots.com/openapi.json" {
		t.Fatalf("OpenAPIURL = %q", cfg.OpenAPIURL)
	}
	if cfg.APIKey != "key-1" {
		t.Fatalf("APIKey = %q, want %q", cfg.APIKey, "key-1")
	}
	if cfg.S3Bucket != "" {
		t.Fatalf("S3Bucket = %q, want empty", cfg.S3Bucket)
	}
	if cfg.S3Prefix != "wherobots-jobs" {
		t.Fatalf("S3Prefix = %q, want %q", cfg.S3Prefix, "wherobots-jobs")
	}
	if cfg.UploadPath != "" {
		t.Fatalf("UploadPath = %q, want empty", cfg.UploadPath)
	}
}

func TestLoadBuildsSpecURLFromWherobotsAPIURL(t *testing.T) {
	t.Setenv("WHEROBOTS_API_URL", "https://api.example.com")
	t.Setenv("WHEROBOTS_API_KEY", "key-1")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.OpenAPIURL != "https://api.example.com/openapi.json" {
		t.Fatalf("OpenAPIURL = %q, want %q", cfg.OpenAPIURL, "https://api.example.com/openapi.json")
	}
}

func TestLoadRequiresWherobotsAPIKey(t *testing.T) {
	t.Setenv("WHEROBOTS_API_URL", "https://api.example.com")
	t.Setenv("WHEROBOTS_API_KEY", "")

	_, err := Load()
	if err == nil {
		t.Fatalf("expected Load() error")
	}
}

func TestLoadReadsS3UploadConfig(t *testing.T) {
	t.Setenv("WHEROBOTS_API_URL", "")
	t.Setenv("WHEROBOTS_API_KEY", "key-1")
	t.Setenv("WHEROBOTS_S3_BUCKET", "bucket-123")
	t.Setenv("WHEROBOTS_S3_PREFIX", "/custom/prefix/")
	t.Setenv("WHEROBOTS_UPLOAD_PATH", "s3://override-bucket/custom/root")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.S3Bucket != "bucket-123" {
		t.Fatalf("S3Bucket = %q, want %q", cfg.S3Bucket, "bucket-123")
	}
	if cfg.S3Prefix != "custom/prefix" {
		t.Fatalf("S3Prefix = %q, want %q", cfg.S3Prefix, "custom/prefix")
	}
	if cfg.UploadPath != "s3://override-bucket/custom/root" {
		t.Fatalf("UploadPath = %q", cfg.UploadPath)
	}
}
