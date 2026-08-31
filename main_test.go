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
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestRun is a regression test for #177: a panic anywhere in the CLI used
// to be recovered and reported, but the process still exited 0, hiding the
// failure from CI. run() is what main() passes to os.Exit, so asserting on
// its return value pins down the real exit code the process will produce.
func TestRun(t *testing.T) {
	t.Run("panic exits non-zero", func(t *testing.T) {
		exitCode := run(func() { panic("boom: something exploded") })
		assert.Equal(t, 1, exitCode)
	})

	t.Run("no panic exits zero", func(t *testing.T) {
		exitCode := run(func() {})
		assert.Equal(t, 0, exitCode)
	})
}

// TestReportPanic verifies the panic message is written to stderr (not
// stdout, which was the previous behaviour) so it doesn't get mixed into a
// command's normal output.
func TestReportPanic(t *testing.T) {
	origStderr := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stderr = w
	defer func() { os.Stderr = origStderr }()

	reportPanic("boom: something exploded")

	_ = w.Close()
	os.Stderr = origStderr

	buf := make([]byte, 4096)
	n, _ := r.Read(buf)
	output := string(buf[:n])

	assert.Contains(t, output, "Something went wrong")
	assert.Contains(t, output, "boom: something exploded")
}
