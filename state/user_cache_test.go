package state

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// useTempCache points os.UserCacheDir at a fresh directory. HOME covers
// darwin ($HOME/Library/Caches); XDG_CACHE_HOME is blanked so linux falls
// back to $HOME/.cache rather than the real one.
//
// t.Setenv means none of these tests can call t.Parallel().
func useTempCache(t *testing.T) {
	t.Helper()

	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", "")
}

// TestWriteToCacheCreatesMissingParent covers a machine where the user cache
// directory does not exist yet -- a slim container, or a fresh CI user. That
// is exactly where the CLI runs during a build.
func TestWriteToCacheCreatesMissingParent(t *testing.T) {
	useTempCache(t)

	if err := WriteToCache("token.json", []byte("secret")); err != nil {
		t.Fatalf("WriteToCache with no pre-existing cache parent: %v", err)
	}

	got, err := ReadFromCache("token.json")
	if err != nil {
		t.Fatalf("ReadFromCache: %v", err)
	}

	if string(got) != "secret" {
		t.Errorf("got %q, want %q", got, "secret")
	}
}

func TestWriteToCacheOverwrites(t *testing.T) {
	useTempCache(t)

	if err := WriteToCache("token.json", []byte("first value, longer")); err != nil {
		t.Fatalf("first WriteToCache: %v", err)
	}

	if err := WriteToCache("token.json", []byte("second")); err != nil {
		t.Fatalf("second WriteToCache: %v", err)
	}

	got, err := ReadFromCache("token.json")
	if err != nil {
		t.Fatalf("ReadFromCache: %v", err)
	}

	// A plain os.Create truncates; without that the tail of the longer first
	// write would still be there.
	if string(got) != "second" {
		t.Errorf("got %q, want %q", got, "second")
	}
}

// TestCachePermissions pins the permissions down because these files hold
// OAuth tokens.
func TestCachePermissions(t *testing.T) {
	useTempCache(t)

	if err := WriteToCache("token.json", []byte("secret")); err != nil {
		t.Fatalf("WriteToCache: %v", err)
	}

	dir, err := CacheDir()
	if err != nil {
		t.Fatalf("CacheDir: %v", err)
	}

	dirInfo, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat cache dir: %v", err)
	}

	if perm := dirInfo.Mode().Perm(); perm != 0o700 {
		t.Errorf("cache dir mode = %#o, want %#o", perm, 0o700)
	}

	fileInfo, err := os.Stat(filepath.Join(dir, "token.json"))
	if err != nil {
		t.Fatalf("stat cache file: %v", err)
	}

	if perm := fileInfo.Mode().Perm(); perm != 0o600 {
		t.Errorf("cache file mode = %#o, want %#o", perm, 0o600)
	}
}

// TestCacheFilePathRejectsEscapes is the guard that keeps a caller from
// steering a token write outside the cache directory.
func TestCacheFilePathRejectsEscapes(t *testing.T) {
	useTempCache(t)

	for _, name := range []string{
		"../escape",
		"../../etc/passwd",
		"sub/dir",
		"/etc/passwd",
		".",
		"..",
		"",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := cacheFilePath(name); err == nil {
				t.Errorf("cacheFilePath(%q) = nil error, want rejection", name)
			}
		})
	}
}

func TestCacheFilePathAcceptsPlainName(t *testing.T) {
	useTempCache(t)

	got, err := cacheFilePath("token.json")
	if err != nil {
		t.Fatalf("cacheFilePath: %v", err)
	}

	dir, err := CacheDir()
	if err != nil {
		t.Fatalf("CacheDir: %v", err)
	}

	if want := filepath.Join(dir, "token.json"); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestWriteToCacheRejectsEscape checks the guard is actually reached through
// the exported entry points, not just present on the helper.
func TestWriteToCacheRejectsEscape(t *testing.T) {
	useTempCache(t)

	if err := WriteToCache("../escape", []byte("secret")); err == nil {
		t.Error("WriteToCache(\"../escape\") = nil error, want rejection")
	}

	if _, err := ReadFromCache("../escape"); err == nil {
		t.Error("ReadFromCache(\"../escape\") = nil error, want rejection")
	}
}

func TestReadFromCacheMissingFile(t *testing.T) {
	useTempCache(t)

	if _, err := ReadFromCache("absent.json"); !os.IsNotExist(err) {
		t.Errorf("got %v, want a not-exist error", err)
	}
}

func TestClearCache(t *testing.T) {
	useTempCache(t)

	if err := WriteToCache("token.json", []byte("secret")); err != nil {
		t.Fatalf("WriteToCache: %v", err)
	}

	if err := ClearCache(); err != nil {
		t.Fatalf("ClearCache: %v", err)
	}

	dir, err := CacheDir()
	if err != nil {
		t.Fatalf("CacheDir: %v", err)
	}

	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("cache dir still present after ClearCache: %v", err)
	}
}

// TestClearCacheWhenAbsent -- logging out twice should not be an error.
func TestClearCacheWhenAbsent(t *testing.T) {
	useTempCache(t)

	if err := ClearCache(); err != nil {
		t.Errorf("ClearCache with no cache present: %v", err)
	}
}

func TestCacheDir(t *testing.T) {
	useTempCache(t)

	dir, err := CacheDir()
	if err != nil {
		t.Fatalf("CacheDir: %v", err)
	}

	if !strings.HasSuffix(dir, cachePrefix) {
		t.Errorf("CacheDir() = %q, want a path ending in %q", dir, cachePrefix)
	}

	if !filepath.IsAbs(dir) {
		t.Errorf("CacheDir() = %q, want an absolute path", dir)
	}
}
