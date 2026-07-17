package selfupdate

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetPlatformInfo(t *testing.T) {
	info, err := GetPlatformInfo()
	require.NoError(t, err)
	assert.NotEmpty(t, info.OS)
	assert.NotEmpty(t, info.Arch)

	// Verify current platform maps correctly
	expectedOS := mapOS(runtime.GOOS)
	expectedArch := mapArch(runtime.GOARCH)
	assert.Equal(t, expectedOS, info.OS)
	assert.Equal(t, expectedArch, info.Arch)
}

func TestMapOS(t *testing.T) {
	tests := []struct {
		goos     string
		expected string
	}{
		{"darwin", "Darwin"},
		{"linux", "Linux"},
		{"windows", "Windows"},
		{"freebsd", ""},
	}

	for _, tt := range tests {
		t.Run(tt.goos, func(t *testing.T) {
			result := mapOS(tt.goos)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMapArch(t *testing.T) {
	tests := []struct {
		goarch   string
		expected string
	}{
		{"amd64", "x86_64"},
		{"386", "i386"},
		{"arm64", "arm64"},
		{"arm", ""},
	}

	for _, tt := range tests {
		t.Run(tt.goarch, func(t *testing.T) {
			result := mapArch(tt.goarch)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetArchiveName(t *testing.T) {
	tests := []struct {
		name     string
		version  string
		platform PlatformInfo
		expected string
	}{
		{
			"Linux amd64 with v prefix",
			"v4.6.7",
			PlatformInfo{"Linux", "x86_64"},
			"apppack_4.6.7_Linux_x86_64.tar.gz",
		},
		{
			"Linux amd64 without v prefix",
			"4.6.7",
			PlatformInfo{"Linux", "x86_64"},
			"apppack_4.6.7_Linux_x86_64.tar.gz",
		},
		{
			"Darwin arm64",
			"v4.6.7",
			PlatformInfo{"Darwin", "arm64"},
			"apppack_4.6.7_Darwin_arm64.tar.gz",
		},
		{
			"Darwin x86_64",
			"v4.6.7",
			PlatformInfo{"Darwin", "x86_64"},
			"apppack_4.6.7_Darwin_x86_64.tar.gz",
		},
		{
			"Windows x86_64",
			"v4.6.7",
			PlatformInfo{"Windows", "x86_64"},
			"apppack_4.6.7_Windows_x86_64.zip",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetArchiveName(tt.version, &tt.platform)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetDownloadURL(t *testing.T) {
	url := GetDownloadURL("v4.6.7", "apppack_4.6.7_Linux_x86_64.tar.gz")
	expected := "https://github.com/apppackio/apppack/releases/download/v4.6.7/apppack_4.6.7_Linux_x86_64.tar.gz"
	assert.Equal(t, expected, url)
}

func TestGetChecksumURL(t *testing.T) {
	url := GetChecksumURL("v4.6.7")
	expected := "https://github.com/apppackio/apppack/releases/download/v4.6.7/checksums.txt"
	assert.Equal(t, expected, url)
}

func TestParseChecksums(t *testing.T) {
	content := []byte(`abc123def456789012345678901234567890123456789012345678901234  apppack_4.6.7_Linux_x86_64.tar.gz
789xyz012345678901234567890123456789012345678901234567890123  apppack_4.6.7_Darwin_arm64.tar.gz
fedcba987654321098765432109876543210987654321098765432109876  apppack_4.6.7_Windows_x86_64.zip
`)

	checksums, err := ParseChecksums(content)
	require.NoError(t, err)

	assert.Len(t, checksums, 3)
	assert.Equal(t, "abc123def456789012345678901234567890123456789012345678901234", checksums["apppack_4.6.7_Linux_x86_64.tar.gz"])
	assert.Equal(t, "789xyz012345678901234567890123456789012345678901234567890123", checksums["apppack_4.6.7_Darwin_arm64.tar.gz"])
	assert.Equal(t, "fedcba987654321098765432109876543210987654321098765432109876", checksums["apppack_4.6.7_Windows_x86_64.zip"])
}

func TestParseChecksumsEmptyLines(t *testing.T) {
	content := []byte(`
abc123  file1.txt

xyz789  file2.txt

`)

	checksums, err := ParseChecksums(content)
	require.NoError(t, err)
	assert.Len(t, checksums, 2)
}

func TestVerifyChecksum(t *testing.T) {
	// Create temp file with known content
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.txt")
	content := []byte("test content for checksum verification")

	err := os.WriteFile(tmpFile, content, 0o644)
	require.NoError(t, err)

	// Compute expected SHA256
	h := sha256.New()
	h.Write(content)
	expected := hex.EncodeToString(h.Sum(nil))

	// Test successful verification
	err = VerifyChecksum(tmpFile, expected)
	assert.NoError(t, err)

	// Test failed verification
	err = VerifyChecksum(tmpFile, "wronghash")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "checksum mismatch")
}

func TestVerifyChecksumFileNotFound(t *testing.T) {
	err := VerifyChecksum("/nonexistent/file.txt", "somehash")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "opening file")
}
