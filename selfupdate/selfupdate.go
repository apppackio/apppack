package selfupdate

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"

	"github.com/apppackio/apppack/state"
	"github.com/apppackio/apppack/version"
	"github.com/google/uuid"
)

const (
	repoOwner = "apppackio"
	repoName  = "apppack"
)

// PlatformInfo represents the current platform for download purposes.
type PlatformInfo struct {
	OS   string // Darwin, Linux, Windows
	Arch string // x86_64, arm64, i386
}

// GetPlatformInfo detects the current OS and architecture,
// mapping to the naming convention used in GoReleaser archives.
func GetPlatformInfo() (*PlatformInfo, error) {
	osName := mapOS(runtime.GOOS)
	if osName == "" {
		return nil, fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
	}

	arch := mapArch(runtime.GOARCH)
	if arch == "" {
		return nil, fmt.Errorf("unsupported architecture: %s", runtime.GOARCH)
	}

	return &PlatformInfo{OS: osName, Arch: arch}, nil
}

func mapOS(goos string) string {
	switch goos {
	case "darwin":
		return "Darwin"
	case "linux":
		return "Linux"
	case "windows":
		return "Windows"
	default:
		return ""
	}
}

func mapArch(goarch string) string {
	switch goarch {
	case "amd64":
		return "x86_64"
	case "386":
		return "i386"
	case "arm64":
		return "arm64"
	default:
		return ""
	}
}

// GetArchiveName constructs the release archive filename.
// Version should be the tag name (e.g., "v4.6.7") - the 'v' prefix is stripped.
func GetArchiveName(ver string, platform *PlatformInfo) string {
	// Strip leading 'v' from version if present
	ver = strings.TrimPrefix(ver, "v")

	ext := ".tar.gz"
	if platform.OS == "Windows" {
		ext = ".zip"
	}

	return fmt.Sprintf("apppack_%s_%s_%s%s", ver, platform.OS, platform.Arch, ext)
}

// GetDownloadURL returns the full download URL for a release archive.
func GetDownloadURL(ver, archiveName string) string {
	return fmt.Sprintf("https://github.com/%s/%s/releases/download/%s/%s",
		repoOwner, repoName, ver, archiveName)
}

// GetChecksumURL returns the URL to checksums.txt for a release version.
func GetChecksumURL(ver string) string {
	return fmt.Sprintf("https://github.com/%s/%s/releases/download/%s/checksums.txt",
		repoOwner, repoName, ver)
}

// DownloadFile downloads a URL to a local file path.
func DownloadFile(ctx context.Context, client *http.Client, url, destPath string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("downloading file: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected HTTP status: %d", resp.StatusCode)
	}

	out, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("creating file: %w", err)
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return fmt.Errorf("writing file: %w", err)
	}

	return nil
}

// DownloadChecksums downloads and parses checksums.txt for a release version.
// Returns a map of filename -> SHA256 hash.
func DownloadChecksums(ctx context.Context, client *http.Client, ver string) (map[string]string, error) {
	url := GetChecksumURL(ver)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("downloading checksums: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected HTTP status: %d", resp.StatusCode)
	}

	content, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading checksums: %w", err)
	}

	return ParseChecksums(content)
}

// ParseChecksums parses checksums.txt content into a filename -> hash map.
// Format: "sha256hash  filename\n" (two spaces between hash and name).
func ParseChecksums(content []byte) (map[string]string, error) {
	checksums := make(map[string]string)
	lines := strings.Split(string(content), "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Format: "hash  filename" (two spaces)
		parts := strings.SplitN(line, "  ", 2)
		if len(parts) != 2 {
			continue
		}

		hash := strings.TrimSpace(parts[0])
		filename := strings.TrimSpace(parts[1])

		if hash != "" && filename != "" {
			checksums[filename] = hash
		}
	}

	return checksums, nil
}

// VerifyChecksum computes SHA256 of a file and compares to expected hash.
func VerifyChecksum(filepath, expected string) error {
	f, err := os.Open(filepath)
	if err != nil {
		return fmt.Errorf("opening file: %w", err)
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return fmt.Errorf("computing hash: %w", err)
	}

	actual := hex.EncodeToString(h.Sum(nil))
	if actual != expected {
		return fmt.Errorf("checksum mismatch: expected %s, got %s", expected, actual)
	}

	return nil
}

// ExtractBinary extracts the apppack binary from an archive.
// Returns the path to the extracted binary.
func ExtractBinary(archivePath, destDir string, platform *PlatformInfo) (string, error) {
	if platform.OS == "Windows" {
		return extractZip(archivePath, destDir)
	}

	return extractTarGz(archivePath, destDir)
}

func extractTarGz(archivePath, destDir string) (string, error) {
	f, err := os.Open(archivePath)
	if err != nil {
		return "", fmt.Errorf("opening archive: %w", err)
	}
	defer f.Close()

	gzr, err := gzip.NewReader(f)
	if err != nil {
		return "", fmt.Errorf("creating gzip reader: %w", err)
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)

	var binaryPath string

	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}

		if err != nil {
			return "", fmt.Errorf("reading tar: %w", err)
		}

		// Look for the apppack binary
		if header.Typeflag == tar.TypeReg && filepath.Base(header.Name) == "apppack" {
			binaryPath = filepath.Join(destDir, "apppack")

			outFile, err := os.OpenFile(binaryPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
			if err != nil {
				return "", fmt.Errorf("creating binary file: %w", err)
			}

			if _, err := io.Copy(outFile, tr); err != nil {
				outFile.Close()

				return "", fmt.Errorf("extracting binary: %w", err)
			}

			outFile.Close()

			break
		}
	}

	if binaryPath == "" {
		return "", errors.New("apppack binary not found in archive")
	}

	return binaryPath, nil
}

func extractZip(archivePath, destDir string) (string, error) {
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return "", fmt.Errorf("opening zip: %w", err)
	}
	defer r.Close()

	var binaryPath string

	for _, f := range r.File {
		// Look for the apppack.exe binary
		if filepath.Base(f.Name) == "apppack.exe" {
			binaryPath = filepath.Join(destDir, "apppack.exe")

			rc, err := f.Open()
			if err != nil {
				return "", fmt.Errorf("opening zip entry: %w", err)
			}

			outFile, err := os.OpenFile(binaryPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
			if err != nil {
				rc.Close()

				return "", fmt.Errorf("creating binary file: %w", err)
			}

			_, err = io.Copy(outFile, rc)
			rc.Close()
			outFile.Close()

			if err != nil {
				return "", fmt.Errorf("extracting binary: %w", err)
			}

			break
		}
	}

	if binaryPath == "" {
		return "", errors.New("apppack.exe binary not found in archive")
	}

	return binaryPath, nil
}

// ReplaceBinary atomically (when possible) replaces the current binary with a new one.
func ReplaceBinary(currentPath, newPath string) error {
	if runtime.GOOS == "windows" {
		return replaceBinaryWindows(currentPath, newPath)
	}

	return replaceBinaryUnix(currentPath, newPath)
}

func replaceBinaryUnix(currentPath, newPath string) error {
	// Get original file info for permissions
	info, err := os.Stat(currentPath)
	if err != nil {
		return fmt.Errorf("getting file info: %w", err)
	}

	// Try atomic rename first
	err = os.Rename(newPath, currentPath)
	if err == nil {
		return os.Chmod(currentPath, info.Mode())
	}

	// Cross-device fallback: copy content
	if errors.Is(err, syscall.EXDEV) {
		return copyAndReplace(newPath, currentPath, info.Mode())
	}

	return fmt.Errorf("replacing binary: %w", err)
}

func copyAndReplace(src, dst string, mode os.FileMode) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("opening source: %w", err)
	}
	defer srcFile.Close()

	// Write to a temp file in the same directory first
	tmpPath := dst + ".new"

	dstFile, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		dstFile.Close()
		os.Remove(tmpPath)

		return fmt.Errorf("copying content: %w", err)
	}

	dstFile.Close()

	// Atomic rename within same filesystem
	if err := os.Rename(tmpPath, dst); err != nil {
		os.Remove(tmpPath)

		return fmt.Errorf("renaming temp file: %w", err)
	}

	return nil
}

func replaceBinaryWindows(currentPath, newPath string) error {
	oldPath := currentPath + ".old"

	// Remove any existing .old file
	os.Remove(oldPath)

	// Rename current to .old
	if err := os.Rename(currentPath, oldPath); err != nil {
		return fmt.Errorf("backing up current binary: %w", err)
	}

	// Copy new binary (can't rename cross-device)
	srcFile, err := os.Open(newPath)
	if err != nil {
		// Attempt rollback
		os.Rename(oldPath, currentPath)

		return fmt.Errorf("opening new binary: %w", err)
	}
	defer srcFile.Close()

	dstFile, err := os.OpenFile(currentPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		// Attempt rollback
		os.Rename(oldPath, currentPath)

		return fmt.Errorf("creating new binary: %w", err)
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		// Attempt rollback
		os.Remove(currentPath)
		os.Rename(oldPath, currentPath)

		return fmt.Errorf("copying new binary: %w", err)
	}

	// Clean up old binary (may fail if still in use, that's ok)
	os.Remove(oldPath)

	return nil
}

// Update performs the complete self-update process.
func Update(ctx context.Context, client *http.Client, release *version.ReleaseInfo, currentBinaryPath string) error {
	platform, err := GetPlatformInfo()
	if err != nil {
		return err
	}

	// Create temp directory for this update
	cacheDir, err := state.CacheDir()
	if err != nil {
		return fmt.Errorf("getting cache directory: %w", err)
	}

	tempDir := filepath.Join(cacheDir, "update-"+uuid.New().String())
	if err := os.MkdirAll(tempDir, 0o700); err != nil {
		return fmt.Errorf("creating temp directory: %w", err)
	}
	// Cleanup on any exit path
	defer os.RemoveAll(tempDir)

	// Download checksums
	checksums, err := DownloadChecksums(ctx, client, release.Version)
	if err != nil {
		return fmt.Errorf("downloading checksums: %w", err)
	}

	// Construct archive name and verify it exists in checksums
	archiveName := GetArchiveName(release.Version, platform)
	expectedChecksum, ok := checksums[archiveName]

	if !ok {
		return fmt.Errorf("no checksum found for %s", archiveName)
	}

	// Download archive
	archivePath := filepath.Join(tempDir, archiveName)
	downloadURL := GetDownloadURL(release.Version, archiveName)

	if err := DownloadFile(ctx, client, downloadURL, archivePath); err != nil {
		return fmt.Errorf("downloading archive: %w", err)
	}

	// Verify checksum
	if err := VerifyChecksum(archivePath, expectedChecksum); err != nil {
		return fmt.Errorf("verifying checksum: %w", err)
	}

	// Extract binary
	newBinaryPath, err := ExtractBinary(archivePath, tempDir, platform)
	if err != nil {
		return fmt.Errorf("extracting binary: %w", err)
	}

	// Replace current binary
	if err := ReplaceBinary(currentBinaryPath, newBinaryPath); err != nil {
		return fmt.Errorf("replacing binary: %w", err)
	}

	return nil
}
