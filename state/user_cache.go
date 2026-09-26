package state

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/sirupsen/logrus"
)

const cachePrefix = "io.apppack"

// cacheFilePath resolves name inside the user cache directory.
//
// name must be a plain filename. Rejecting anything else stops a caller from
// walking out of the cache directory, which matters here because these files
// hold OAuth tokens.
func cacheFilePath(name string) (string, error) {
	if name != filepath.Base(name) || name == "." || name == ".." {
		return "", fmt.Errorf("invalid cache entry name %q", name)
	}

	dir, err := CacheDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(dir, name), nil
}

func WriteToCache(name string, data []byte) (err error) {
	path, err := CacheDir()
	if err != nil {
		return err
	}

	filename, err := cacheFilePath(name)
	if err != nil {
		return err
	}

	// MkdirAll, not Mkdir: on a machine where the user cache directory does
	// not exist yet -- a slim container, a fresh CI user -- the parent has to
	// be created too, and it already returns nil when the directory is there.
	err = os.MkdirAll(path, os.FileMode(0o700))
	if err != nil {
		return err
	}

	logrus.WithFields(logrus.Fields{"filename": filename}).Debug("writing to user cache")

	file, err := os.Create(filename) // #nosec G304 -- name is validated by cacheFilePath
	if err != nil {
		return err
	}

	// A failed Close on a written file can mean the data never reached disk,
	// so it replaces a nil error rather than being dropped.
	defer func() {
		if closeErr := file.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
	}()

	err = file.Chmod(os.FileMode(0o600))
	if err != nil {
		return err
	}

	_, err = file.Write(data)

	return err
}

func ReadFromCache(name string) ([]byte, error) {
	filename, err := cacheFilePath(name)
	if err != nil {
		return nil, err
	}

	logrus.WithFields(logrus.Fields{"filename": filename}).Debug("reading from user cache")

	file, err := os.Open(filename) // #nosec G304 -- name is validated by cacheFilePath
	if err != nil {
		return nil, err
	}

	// Read-only: a Close failure here tells the caller nothing actionable.
	defer func() { _ = file.Close() }()

	return io.ReadAll(file)
}

func ClearCache() error {
	path, err := CacheDir()
	if err != nil {
		return err
	}

	logrus.WithFields(logrus.Fields{"path": path}).Debug("deleting user cache")

	return os.RemoveAll(path)
}

func CacheDir() (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(dir, cachePrefix), nil
}
