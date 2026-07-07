package auth

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func testSession(access string) Session {
	return Session{
		AccessToken:   access,
		RefreshToken:  "refresh-1",
		ExpiresAt:     time.Now().Add(time.Hour).UTC().Truncate(time.Second),
		TokenEndpoint: "https://login.example/oauth2/token",
		ClientID:      "client_test_123",
		Email:         "clay@wherobots.com",
		Sub:           "user_01ABC",
	}
}

func TestStoreRoundTrip(t *testing.T) {
	t.Parallel()
	store := &Store{Path: filepath.Join(t.TempDir(), "nested", "credentials.json")}

	want := testSession("access-1")
	if err := store.Put("https://login.example", want); err != nil {
		t.Fatalf("Put() error = %v", err)
	}

	got, err := store.Get("https://login.example")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got == nil || *got != want {
		t.Fatalf("Get() = %+v, want %+v", got, want)
	}
}

func TestStoreGetMissingFileReturnsNil(t *testing.T) {
	t.Parallel()
	store := &Store{Path: filepath.Join(t.TempDir(), "credentials.json")}

	got, err := store.Get("https://login.example")
	if err != nil || got != nil {
		t.Fatalf("Get() = %+v, %v; want nil, nil", got, err)
	}
}

func TestStoreDomainsDoNotClobber(t *testing.T) {
	t.Parallel()
	store := &Store{Path: filepath.Join(t.TempDir(), "credentials.json")}

	prod := testSession("prod-access")
	staging := testSession("staging-access")
	if err := store.Put("https://login.prod", prod); err != nil {
		t.Fatalf("Put(prod) error = %v", err)
	}
	if err := store.Put("https://login.staging", staging); err != nil {
		t.Fatalf("Put(staging) error = %v", err)
	}

	gotProd, err := store.Get("https://login.prod")
	if err != nil || gotProd == nil || gotProd.AccessToken != "prod-access" {
		t.Fatalf("Get(prod) = %+v, %v", gotProd, err)
	}
	gotStaging, err := store.Get("https://login.staging")
	if err != nil || gotStaging == nil || gotStaging.AccessToken != "staging-access" {
		t.Fatalf("Get(staging) = %+v, %v", gotStaging, err)
	}
}

func TestStorePermissions(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("unix permissions are advisory-only on windows")
	}
	dir := filepath.Join(t.TempDir(), "wherobots")
	store := &Store{Path: filepath.Join(dir, "credentials.json")}

	if err := store.Put("https://login.example", testSession("a")); err != nil {
		t.Fatalf("Put() error = %v", err)
	}

	dirInfo, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat dir: %v", err)
	}
	if perm := dirInfo.Mode().Perm(); perm != 0o700 {
		t.Errorf("dir perm = %o, want 700", perm)
	}
	fileInfo, err := os.Stat(store.Path)
	if err != nil {
		t.Fatalf("stat file: %v", err)
	}
	if perm := fileInfo.Mode().Perm(); perm != 0o600 {
		t.Errorf("file perm = %o, want 600", perm)
	}
}

func TestStoreCorruptFile(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "credentials.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatalf("write corrupt file: %v", err)
	}
	store := &Store{Path: path}

	_, err := store.Get("https://login.example")
	if !errors.Is(err, ErrCorruptStore) {
		t.Fatalf("err = %v, want ErrCorruptStore", err)
	}
}

func TestStoreDelete(t *testing.T) {
	t.Parallel()
	store := &Store{Path: filepath.Join(t.TempDir(), "credentials.json")}

	if err := store.Put("https://login.a", testSession("a")); err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	if err := store.Put("https://login.b", testSession("b")); err != nil {
		t.Fatalf("Put() error = %v", err)
	}

	removed, err := store.Delete("https://login.a")
	if err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if removed == nil || removed.AccessToken != "a" {
		t.Fatalf("Delete() removed = %+v", removed)
	}

	if got, _ := store.Get("https://login.a"); got != nil {
		t.Errorf("session a still present after delete")
	}
	if got, _ := store.Get("https://login.b"); got == nil {
		t.Errorf("session b removed by unrelated delete")
	}

	// Deleting a missing domain is a no-op reported via nil.
	removed, err = store.Delete("https://login.missing")
	if err != nil || removed != nil {
		t.Fatalf("Delete(missing) = %+v, %v; want nil, nil", removed, err)
	}
}

func TestStoreDeleteAll(t *testing.T) {
	t.Parallel()
	store := &Store{Path: filepath.Join(t.TempDir(), "credentials.json")}

	_ = store.Put("https://login.a", testSession("a"))
	_ = store.Put("https://login.b", testSession("b"))

	count, err := store.DeleteAll()
	if err != nil {
		t.Fatalf("DeleteAll() error = %v", err)
	}
	if count != 2 {
		t.Fatalf("DeleteAll() count = %d, want 2", count)
	}
	if got, _ := store.Get("https://login.a"); got != nil {
		t.Errorf("session a still present after DeleteAll")
	}
}

func TestStoreWriteLeavesNoTempFiles(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	store := &Store{Path: filepath.Join(dir, "credentials.json")}

	if err := store.Put("https://login.example", testSession("a")); err != nil {
		t.Fatalf("Put() error = %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "credentials.json" {
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Fatalf("dir entries = %v, want only credentials.json", names)
	}
}

func TestStoreEmptyPathReportsConfigDirProblem(t *testing.T) {
	t.Parallel()
	// config.Load leaves Path empty when the user config dir is unresolvable;
	// the store must surface an actionable error instead of touching "".
	store := &Store{}
	if _, err := store.Get("https://login.example"); err == nil || !strings.Contains(err.Error(), "config directory") {
		t.Fatalf("Get() err = %v, want config-dir guidance", err)
	}
	if err := store.Put("https://login.example", testSession("access-1")); err == nil || !strings.Contains(err.Error(), "config directory") {
		t.Fatalf("Put() err = %v, want config-dir guidance", err)
	}
}
