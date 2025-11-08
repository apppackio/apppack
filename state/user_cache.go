package state

import (
	"io"
	"os"
	"path/filepath"

	"github.com/sirupsen/logrus"
)

const cachePrefix = "io.apppack"

func WriteToCache(name string, data []byte) error {
	path, err := CacheDir()
	if err != nil {
		return err
	}

	err = os.Mkdir(path, os.FileMode(0o700))
	if err != nil {
		if !os.IsExist(err) {
			return err
		}
	}

	filename := filepath.Join(path, name)
	logrus.WithFields(logrus.Fields{"filename": filename}).Debug("writing to user cache")
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()
	err = file.Chmod(os.FileMode(0o600))
	if err != nil {
		return err
	}
	_, err = file.Write(data)

	return err
}

func ReadFromCache(name string) ([]byte, error) {
	path, err := CacheDir()
	if err != nil {
		return nil, err
	}

	filename := filepath.Join(path, name)
	logrus.WithFields(logrus.Fields{"filename": filename}).Debug("reading from user cache")
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

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
