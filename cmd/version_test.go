package cmd

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/apppackio/apppack/version"
)

// withVersion swaps the build-stamped package variables for the duration of a
// test. They are set by ldflags at release time, so a test has to stand in for
// the linker.
func withVersion(t *testing.T, v, environment string) {
	t.Helper()

	origVersion, origEnv := version.Version, version.Environment

	t.Cleanup(func() {
		version.Version, version.Environment = origVersion, origEnv
	})

	version.Version, version.Environment = v, environment
}

func TestVersionPrintsTheVersion(t *testing.T) {
	withVersion(t, "v4.8.4", "production")

	got := run(t, "version")
	if got.err != nil {
		t.Fatalf("version: %v", got.err)
	}

	if strings.TrimSpace(got.stdout) != "v4.8.4" {
		t.Errorf("stdout = %q, want v4.8.4", got.stdout)
	}
}

// A binary built outside a release has no version stamped in, so it reports
// which environment it was built for instead.
func TestVersionPrintsTheEnvironmentWhenNotProduction(t *testing.T) {
	withVersion(t, "v4.8.4", "development")

	got := run(t, "version")
	if got.err != nil {
		t.Fatalf("version: %v", got.err)
	}

	if strings.TrimSpace(got.stdout) != "development" {
		t.Errorf("stdout = %q, want development", got.stdout)
	}
}

func TestVersionJSON(t *testing.T) {
	withVersion(t, "v4.8.4", "production")

	got := run(t, "version", "--json")
	if got.err != nil {
		t.Fatalf("version --json: %v", got.err)
	}

	var info versionInfo
	if err := json.Unmarshal([]byte(got.stdout), &info); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, got.stdout)
	}

	if info.Version != "v4.8.4" {
		t.Errorf("version = %q, want v4.8.4", info.Version)
	}

	if info.Environment != "production" {
		t.Errorf("environment = %q, want production", info.Environment)
	}
}

// --json wins over the environment rule: a development build still produces
// parseable JSON rather than a bare word.
func TestVersionJSONInDevelopment(t *testing.T) {
	withVersion(t, "v4.8.4", "development")

	got := run(t, "version", "--json")
	if got.err != nil {
		t.Fatalf("version --json: %v", got.err)
	}

	var info versionInfo
	if err := json.Unmarshal([]byte(got.stdout), &info); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, got.stdout)
	}

	if info.Environment != "development" {
		t.Errorf("environment = %q, want development", info.Environment)
	}
}

func TestReportUpdateAvailable(t *testing.T) {
	withVersion(t, "v4.8.0", "production")

	tests := []struct {
		name     string
		latest   string
		want     string
		wantNoop string
	}{
		{name: "newer release", latest: "v4.9.0", want: "Update available:"},
		{name: "same release", latest: "v4.8.0", want: "Already up to date"},
		{name: "older release", latest: "v4.7.0", want: "Already up to date"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer

			if err := reportUpdateAvailable(&out, tt.latest); err != nil {
				t.Fatalf("reportUpdateAvailable: %v", err)
			}

			if !strings.Contains(out.String(), tt.want) {
				t.Errorf("output = %q, want it to contain %q", out.String(), tt.want)
			}
		})
	}
}

// The leading "v" is trimmed for display but has to survive the comparison,
// so "v4.10.0" must read as newer than "v4.9.0" rather than sorting as text.
func TestReportUpdateAvailableComparesNumerically(t *testing.T) {
	withVersion(t, "v4.9.0", "production")

	var out bytes.Buffer

	if err := reportUpdateAvailable(&out, "v4.10.0"); err != nil {
		t.Fatalf("reportUpdateAvailable: %v", err)
	}

	if !strings.Contains(out.String(), "Update available:") {
		t.Errorf("v4.10.0 did not read as newer than v4.9.0: %q", out.String())
	}
}

func TestVersionSubcommandsAreRegistered(t *testing.T) {
	got := run(t, "version", "--help")
	if got.err != nil {
		t.Fatalf("version --help: %v", got.err)
	}

	for _, want := range []string{"check", "update"} {
		if !strings.Contains(got.stdout, want) {
			t.Errorf("help is missing the %q subcommand:\n%s", want, got.stdout)
		}
	}
}

func TestVersionUpdateForceFlag(t *testing.T) {
	got := run(t, "version", "update", "--help")
	if got.err != nil {
		t.Fatalf("version update --help: %v", got.err)
	}

	if !strings.Contains(got.stdout, "--force") {
		t.Errorf("help is missing --force:\n%s", got.stdout)
	}
}
