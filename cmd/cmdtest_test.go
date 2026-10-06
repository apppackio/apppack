package cmd

import (
	"bytes"
	"testing"

	"github.com/spf13/cobra"
)

// newTestRootCmd returns a root command carrying its own copy of every
// command that has been converted to a constructor. Commands still registered
// from an init() are deliberately absent: they are singletons, so including
// them would let one test's flag values show up in the next.
func newTestRootCmd() *cobra.Command {
	root := newRootCmd()
	for _, newCmd := range commandFactories {
		root.AddCommand(newCmd())
	}

	return root
}

// runResult is what a command left behind.
type runResult struct {
	stdout string
	stderr string
	err    error
}

// run executes args against a fresh tree and captures its output.
//
// Anything written through cmd.OutOrStdout()/ErrOrStderr() is captured.
// Anything a command prints with a bare fmt.Println still goes to the real
// stdout -- converting those is part of each command's own PR.
func run(t *testing.T, args ...string) runResult {
	t.Helper()

	var stdout, stderr bytes.Buffer

	root := newTestRootCmd()
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs(args)

	err := root.Execute()

	return runResult{stdout: stdout.String(), stderr: stderr.String(), err: err}
}
