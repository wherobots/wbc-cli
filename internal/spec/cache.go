package spec

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type cacheMetadata struct {
	FetchedAt time.Time `json:"fetched_at"`
	SourceURL string    `json:"source_url"`
}

type cacheState struct {
	SpecBytes []byte
	Meta      cacheMetadata
}

func readCache(specPath, metaPath string) (cacheState, error) {
	specBytes, err := os.ReadFile(specPath)
	if err != nil {
		return cacheState{}, err
	}

	meta := cacheMetadata{}
	metaBytes, err := os.ReadFile(metaPath)
	if err == nil {
		if unmarshalErr := json.Unmarshal(metaBytes, &meta); unmarshalErr != nil {
			return cacheState{}, fmt.Errorf("parse cache metadata: %w", unmarshalErr)
		}
	}

	if meta.FetchedAt.IsZero() {
		stat, statErr := os.Stat(specPath)
		if statErr == nil {
			meta.FetchedAt = stat.ModTime().UTC()
		}
	}

	return cacheState{SpecBytes: specBytes, Meta: meta}, nil
}

func writeCache(specPath, metaPath string, specBytes []byte, sourceURL string, now time.Time) error {
	if err := os.MkdirAll(filepath.Dir(specPath), 0o755); err != nil {
		return fmt.Errorf("create cache dir: %w", err)
	}

	if err := os.WriteFile(specPath, specBytes, 0o644); err != nil {
		return fmt.Errorf("write spec cache: %w", err)
	}

	meta := cacheMetadata{FetchedAt: now.UTC(), SourceURL: sourceURL}
	metaBytes, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("marshal cache metadata: %w", err)
	}
	if err := os.WriteFile(metaPath, metaBytes, 0o644); err != nil {
		return fmt.Errorf("write cache metadata: %w", err)
	}
	return nil
}

func cacheExists(specPath string) bool {
	_, err := os.Stat(specPath)
	return err == nil
}

func isFresh(meta cacheMetadata, ttl time.Duration, now time.Time) bool {
	if ttl <= 0 || meta.FetchedAt.IsZero() {
		return false
	}
	return now.UTC().Sub(meta.FetchedAt.UTC()) < ttl
}

func isMissing(err error) bool {
	return errors.Is(err, os.ErrNotExist)
}
