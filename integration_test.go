package main_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/apppackio/apppack/cmd"
	"github.com/rogpeppe/go-internal/testscript"
)

func TestMain(m *testing.M) {
	testscript.Main(m, map[string]func(){
		"apppack": cmd.Execute,
	})
}

func TestCLI(t *testing.T) {
	testscript.Run(t, testscript.Params{
		Dir: "testdata/script",
		Setup: func(env *testscript.Env) error {
			env.Setenv("NO_COLOR", "1")

			// Every `exec apppack` re-runs this test binary, so under
			// `go test -cover` each one emits coverage on exit. They
			// inherit GOCOVERDIR from the parent and scripts run in
			// parallel, which trips a race in the Go runtime: the
			// meta-data temp file is named from time.Now().UnixNano()
			// with no PID (unlike the counter file, which has one).
			// macOS only gives that clock microsecond resolution, so two
			// processes exiting together pick the same temp name, and the
			// loser's rename fails with ENOENT. The error goes to stderr,
			// failing whichever script's `! stderr .` happened to be
			// running -- a different one each time.
			//
			// A per-script directory keeps the processes off each other's
			// temp names. Nothing is lost by not writing into the
			// directory `go test` collects: the subprocesses run
			// cmd.Execute, so they contribute no coverage of this
			// package. The reported number is unchanged.
			covdir := filepath.Join(env.WorkDir, "gocoverdir")
			if err := os.MkdirAll(covdir, 0o700); err != nil {
				return err
			}
			env.Setenv("GOCOVERDIR", covdir)

			return nil
		},
	})
}
