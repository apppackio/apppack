package cmd_test

import (
	"fmt"
	"testing"

	"github.com/apppackio/apppack/cmd"
)

func TestLogFlagConversionForSaw(t *testing.T) {
	// valid, just need the "-" prefix
	for _, v := range []string{"1h", "50m", "3600s"} {
		val, err := cmd.TimeValForSaw(v)
		if val != fmt.Sprintf("-%s", v) {
			t.Errorf("Expected -%s, got %s", v, val)
		}
		if err != nil {
			t.Errorf("Expected no error, got %s", err)
		}
	}

	// needs conversion to hours *and* "-" prefix
	val, err := cmd.TimeValForSaw("2d")
	if val != "-48h" {
		t.Errorf("Expected -48h, got %s", val)
	}
	if err != nil {
		t.Errorf("Expected no error, got %s", err)
	}

	// invalid
	for _, v := range []string{"2w", "20220101", "12:30"} {
		_, err := cmd.TimeValForSaw(v)
		if err == nil {
			t.Errorf("Expected error for %s, got nil", v)
		}
	}

	// valid absolute times
	for _, v := range []string{"2022-12-01T14:52:00+00:00", "2022-12-01T14:52:00-06:00", "2022-12-01T14:52:00Z"} {
		val, err := cmd.TimeValForSaw(v)
		if val != v {
			t.Errorf("Expected %s, got %s", v, val)
		}
		if err != nil {
			t.Errorf("Expected no error, got %s", err)
		}
	}
}
