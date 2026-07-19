package app_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/apppackio/apppack/app"
)

func TestErrDDBItemNotFoundUnwraps(t *testing.T) {
	t.Parallel()

	wrapped := fmt.Errorf("could not find DDB item %s %s: %w", "APP#foo", "DEPLOYSTATUS", app.ErrDDBItemNotFound)

	if !errors.Is(wrapped, app.ErrDDBItemNotFound) {
		t.Error("expected errors.Is to match app.ErrDDBItemNotFound through the wrapped message")
	}

	otherErr := errors.New("some other ddb error")
	if errors.Is(otherErr, app.ErrDDBItemNotFound) {
		t.Error("expected an unrelated error not to match app.ErrDDBItemNotFound")
	}
}
