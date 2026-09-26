package bridge_test

import (
	"testing"

	"github.com/apppackio/apppack/bridge"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
)

func TestGetStackParameter(t *testing.T) {
	t.Parallel()

	params := []types.Parameter{
		{ParameterKey: aws.String("Name"), ParameterValue: aws.String("myapp")},
		{ParameterKey: aws.String("Empty"), ParameterValue: aws.String("")},
	}

	t.Run("found", func(t *testing.T) {
		t.Parallel()

		got, err := bridge.GetStackParameter(params, "Name")
		if err != nil {
			t.Fatalf("GetStackParameter: %v", err)
		}

		if *got != "myapp" {
			t.Errorf("got %q, want %q", *got, "myapp")
		}
	})

	// An empty value is a value. Returning an error here would make callers
	// treat a deliberately blank parameter as missing.
	t.Run("found but empty", func(t *testing.T) {
		t.Parallel()

		got, err := bridge.GetStackParameter(params, "Empty")
		if err != nil {
			t.Fatalf("GetStackParameter: %v", err)
		}

		if *got != "" {
			t.Errorf("got %q, want empty string", *got)
		}
	})

	t.Run("missing", func(t *testing.T) {
		t.Parallel()

		_, err := bridge.GetStackParameter(params, "Absent")
		if err == nil {
			t.Fatal("got nil error for a parameter that is not there")
		}

		if err.Error() != "no parameter named Absent" {
			t.Errorf("error = %q", err.Error())
		}
	})

	t.Run("empty slice", func(t *testing.T) {
		t.Parallel()

		if _, err := bridge.GetStackParameter(nil, "Name"); err == nil {
			t.Error("got nil error searching an empty parameter list")
		}
	})
}

func TestGetStackOutput(t *testing.T) {
	t.Parallel()

	outputs := []types.Output{
		{OutputKey: aws.String("Url"), OutputValue: aws.String("https://example.com")},
		{OutputKey: aws.String("Empty"), OutputValue: aws.String("")},
	}

	t.Run("found", func(t *testing.T) {
		t.Parallel()

		got, err := bridge.GetStackOutput(outputs, "Url")
		if err != nil {
			t.Fatalf("GetStackOutput: %v", err)
		}

		if *got != "https://example.com" {
			t.Errorf("got %q", *got)
		}
	})

	t.Run("found but empty", func(t *testing.T) {
		t.Parallel()

		got, err := bridge.GetStackOutput(outputs, "Empty")
		if err != nil {
			t.Fatalf("GetStackOutput: %v", err)
		}

		if *got != "" {
			t.Errorf("got %q, want empty string", *got)
		}
	})

	t.Run("missing", func(t *testing.T) {
		t.Parallel()

		_, err := bridge.GetStackOutput(outputs, "Absent")
		if err == nil {
			t.Fatal("got nil error for an output that is not there")
		}

		if err.Error() != "no output named Absent" {
			t.Errorf("error = %q", err.Error())
		}
	})

	t.Run("empty slice", func(t *testing.T) {
		t.Parallel()

		if _, err := bridge.GetStackOutput(nil, "Url"); err == nil {
			t.Error("got nil error searching an empty output list")
		}
	})
}
