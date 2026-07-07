package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// ErrCorruptStore marks an unreadable credentials file. It is never
// auto-deleted; `wherobots auth login` overwrites it.
var ErrCorruptStore = errors.New("credentials file is corrupt")

// errNoConfigDir surfaces lazily: config.Load tolerates an unresolvable user
// config dir (e.g. HOME unset in CI) so API-key-only usage keeps working;
// the error only matters once the credentials file is actually needed.
var errNoConfigDir = errors.New("cannot locate the credentials file: the user config directory is not resolvable (set HOME or XDG_CONFIG_HOME)")

// Session is one stored OAuth session, keyed by AuthKit domain so prod and
// staging sign-ins coexist in the same file.
type Session struct {
	AccessToken   string    `json:"access_token"`
	RefreshToken  string    `json:"refresh_token,omitempty"`
	ExpiresAt     time.Time `json:"expires_at"`
	TokenEndpoint string    `json:"token_endpoint,omitempty"`
	ClientID      string    `json:"client_id,omitempty"`
	Email         string    `json:"email,omitempty"`
	Sub           string    `json:"sub,omitempty"`
}

type storeFile struct {
	Version  int                `json:"version"`
	Sessions map[string]Session `json:"sessions"`
}

// Store persists OAuth sessions as a single 0600 JSON file. Writes are
// whole-file read-modify-write with an atomic temp+rename so a partial write
// can never strand a half-written file.
type Store struct {
	Path string
}

// Get returns the session for a domain, (nil, nil) when absent.
func (s *Store) Get(domain string) (*Session, error) {
	file, err := s.read()
	if err != nil {
		return nil, err
	}
	session, ok := file.Sessions[domain]
	if !ok {
		return nil, nil
	}
	return &session, nil
}

// Put saves the session for a domain, preserving other domains' sessions.
func (s *Store) Put(domain string, session Session) error {
	file, err := s.read()
	if err != nil && !errors.Is(err, ErrCorruptStore) {
		return err
	}
	if file == nil || errors.Is(err, ErrCorruptStore) {
		file = &storeFile{Version: 1, Sessions: map[string]Session{}}
	}
	file.Sessions[domain] = session
	return s.write(file)
}

// Delete removes a domain's session and returns it, or (nil, nil) when there
// was none.
func (s *Store) Delete(domain string) (*Session, error) {
	file, err := s.read()
	if err != nil {
		return nil, err
	}
	session, ok := file.Sessions[domain]
	if !ok {
		return nil, nil
	}
	delete(file.Sessions, domain)
	if err := s.write(file); err != nil {
		return nil, err
	}
	return &session, nil
}

// DeleteAll removes every stored session and reports how many were removed.
func (s *Store) DeleteAll() (int, error) {
	file, err := s.read()
	if err != nil {
		return 0, err
	}
	count := len(file.Sessions)
	if count == 0 {
		return 0, nil
	}
	file.Sessions = map[string]Session{}
	if err := s.write(file); err != nil {
		return 0, err
	}
	return count, nil
}

// read loads the store; a missing file yields an empty store.
func (s *Store) read() (*storeFile, error) {
	if s.Path == "" {
		return nil, errNoConfigDir
	}
	data, err := os.ReadFile(s.Path)
	if errors.Is(err, os.ErrNotExist) {
		return &storeFile{Version: 1, Sessions: map[string]Session{}}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read credentials file: %w", err)
	}

	var file storeFile
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("%w (%s)", ErrCorruptStore, s.Path)
	}
	if file.Sessions == nil {
		file.Sessions = map[string]Session{}
	}
	return &file, nil
}

// write persists the store atomically with owner-only permissions.
func (s *Store) write(file *storeFile) error {
	if s.Path == "" {
		return errNoConfigDir
	}
	dir := filepath.Dir(s.Path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create credentials dir: %w", err)
	}

	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return fmt.Errorf("encode credentials: %w", err)
	}

	tmp, err := os.CreateTemp(dir, ".credentials-*.json")
	if err != nil {
		return fmt.Errorf("write credentials: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return fmt.Errorf("write credentials: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write credentials: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("write credentials: %w", err)
	}
	if err := os.Rename(tmpName, s.Path); err != nil {
		return fmt.Errorf("write credentials: %w", err)
	}
	return nil
}
