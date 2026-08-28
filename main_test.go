/*
Copyright © 2020 NAME HERE <EMAIL ADDRESS>

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package main

import (
	"bytes"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// crashAndRecover mirrors what main() wires up: handlePanic deferred as the
// outermost recover around some code that panics.
func crashAndRecover() {
	defer handlePanic()
	panic("boom: something exploded")
}

// TestHandlePanicSubprocess is a regression test for #177: the CLI used to
// recover from panics and still exit 0, hiding failures from CI. It
// re-execs this same test binary in a subprocess (BE_CRASHER=1) so it can
// observe the real process exit code produced by os.Exit, which can't be
// captured within the test process itself.
func TestHandlePanicSubprocess(t *testing.T) {
	if os.Getenv("BE_CRASHER") == "1" {
		crashAndRecover()
		return
	}

	// #nosec G204 -- os.Args[0] is this test binary, not user input.
	cmd := exec.Command(os.Args[0], "-test.run=TestHandlePanicSubprocess")
	cmd.Env = append(os.Environ(), "BE_CRASHER=1")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	var exitErr *exec.ExitError
	require.ErrorAs(t, err, &exitErr, "expected process to exit with an error, got: %v", err)
	assert.Equal(t, 1, exitErr.ExitCode(), "a panic must exit non-zero so CI can detect it")

	// showCursor() writes a terminal escape sequence to stdout (matching the
	// SIGTERM path), but the panic message itself must go to stderr.
	assert.False(t, strings.Contains(stdout.String(), "Something went wrong"), "panic message should not be written to stdout, got: %q", stdout.String())
	assert.True(t, strings.Contains(stderr.String(), "Something went wrong"), "expected panic message on stderr, got: %q", stderr.String())
}
