package main_test

import (
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
			return nil
		},
	})
}
