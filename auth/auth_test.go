package auth

import (
	"errors"
	"strings"
	"testing"
)

func TestFriendlyAWSConfigError(t *testing.T) {
	t.Run("nil error stays nil", func(t *testing.T) {
		if err := FriendlyAWSConfigError(nil); err != nil {
			t.Fatalf("expected nil, got %v", err)
		}
	})

	t.Run("partial credentials error gets a hint", func(t *testing.T) {
		raw := errors.New("error fetching config from profile, default, Error using profile: \n 2, partial credentials found for profile default")
		got := FriendlyAWSConfigError(raw)
		if got == nil {
			t.Fatal("expected wrapped error, got nil")
		}
		if !strings.Contains(got.Error(), "local AWS credentials appear to be incomplete") {
			t.Errorf("expected friendly hint, got: %v", got)
		}
		if !errors.Is(got, raw) {
			t.Error("expected wrapped error to preserve the original via errors.Is")
		}
	})

	t.Run("unrelated error is passed through unchanged", func(t *testing.T) {
		raw := errors.New("some other failure")
		got := FriendlyAWSConfigError(raw)
		if !errors.Is(got, raw) || got.Error() != raw.Error() {
			t.Errorf("expected passthrough, got: %v", got)
		}
	})
}
