package cmd

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// TestOpenRequiresAppName -- cobra rejects this before RunE is reached, so it
// is checkable without AWS. Under the old `Run:` + checkErr shape the process
// would have exited instead of returning, and there would have been nothing
// for a test to assert on.
func TestOpenRequiresAppName(t *testing.T) {
	got := run(t, "open")

	if got.err == nil {
		t.Fatal("got nil error running open with no --app-name")
	}

	if !strings.Contains(got.err.Error(), "app-name") {
		t.Errorf("error = %q, want it to name the missing flag", got.err)
	}
}

func TestOpenRejectsUnknownFlag(t *testing.T) {
	got := run(t, "open", "--app-name", "myapp", "--nope")

	if got.err == nil {
		t.Fatal("got nil error for an unknown flag")
	}

	if !strings.Contains(got.err.Error(), "nope") {
		t.Errorf("error = %q, want it to name the unknown flag", got.err)
	}
}

// TestOpenUsageDoesNotFollowErrors -- SilenceUsage is what keeps a runtime
// failure from being buried under the whole usage block.
func TestOpenUsageDoesNotFollowErrors(t *testing.T) {
	got := run(t, "open")

	if strings.Contains(got.stdout, "Usage:") || strings.Contains(got.stderr, "Usage:") {
		t.Errorf("usage was printed after an error:\nstdout: %s\nstderr: %s", got.stdout, got.stderr)
	}
}

func TestOpenHelp(t *testing.T) {
	got := run(t, "open", "--help")

	if got.err != nil {
		t.Fatalf("open --help: %v", got.err)
	}

	for _, want := range []string{"open the app in a browser", "--app-name", "--aws-credentials"} {
		if !strings.Contains(got.stdout, want) {
			t.Errorf("help output is missing %q:\n%s", want, got.stdout)
		}
	}
}

// TestCommandTreesAreIndependent is the property the harness exists for. Two
// trees hold two openOptions, so a flag set on one is invisible to the other.
// Before the conversion both would have written to the package-level AppName.
func TestCommandTreesAreIndependent(t *testing.T) {
	first, second := newTestRootCmd(), newTestRootCmd()

	openOf := func(root *cobra.Command) *cobra.Command {
		t.Helper()

		for _, c := range root.Commands() {
			if c.Name() == "open" {
				return c
			}
		}

		t.Fatal("open command is not on the tree")

		return nil
	}

	firstOpen, secondOpen := openOf(first), openOf(second)

	if err := firstOpen.PersistentFlags().Set("app-name", "myapp"); err != nil {
		t.Fatalf("setting app-name: %v", err)
	}

	if got := firstOpen.PersistentFlags().Lookup("app-name").Value.String(); got != "myapp" {
		t.Fatalf("first tree app-name = %q, want myapp", got)
	}

	if got := secondOpen.PersistentFlags().Lookup("app-name").Value.String(); got != "" {
		t.Errorf("second tree app-name = %q, want it untouched", got)
	}
}
