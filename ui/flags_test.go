package ui_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/apppackio/apppack/ui"
	"github.com/spf13/pflag"
)

func TestFlagsToStruct(t *testing.T) {
	type TestStruct struct {
		Strsl []string `flag:"strsl"`
	}
	data := []string{"a", "b", "c"}
	s := TestStruct{}
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	fs.StringSlice("strsl", []string{}, "")
	fs.Parse([]string{
		fmt.Sprintf("--strsl=%s", strings.Join(data, ",")),
	})
	err := ui.FlagsToStruct(&s, fs)
	if err != nil {
		t.Errorf("Error converting struct to flags: %s", err)
	}
	if len(s.Strsl) != 3 {
		t.Errorf("Expected 3 strings in strsl, got %d", len(s.Strsl))
	}
	for i, v := range data {
		if s.Strsl[i] != v {
			t.Errorf("Expected %s in strsl, got %s", v, s.Strsl[i])
		}
	}
}
