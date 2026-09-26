package cmd

import (
	"errors"
	"strings"
	"testing"

	"github.com/apppackio/apppack/app"
)

// TestBuildPhaseFailedError verifies the error text is unchanged from the
// plain fmt.Errorf it replaced, and that errors.As only matches this type --
// not a plain AWS/CLI error -- so the diagnose hint is only ever offered for
// an actual build phase failure.
func TestBuildPhaseFailedError(t *testing.T) {
	t.Parallel()

	err := error(&buildPhaseFailedError{phase: "Deploy", at: "Sep 24, 2026 10:15:00 MDT"})

	const want = "Deploy failed at Sep 24, 2026 10:15:00 MDT"
	if err.Error() != want {
		t.Errorf("expected error text %q, got %q", want, err.Error())
	}

	var phaseErr *buildPhaseFailedError
	if !errors.As(err, &phaseErr) {
		t.Error("expected errors.As to match buildPhaseFailedError")
	}

	plain := errors.New("connection refused")

	var notPhaseErr *buildPhaseFailedError
	if errors.As(plain, &notPhaseErr) {
		t.Error("expected errors.As to NOT match a plain error")
	}
}

// TestReportWatchBuildErr_PhaseFailure verifies that a build phase failure
// prints the failure line before the diagnose hint -- the ordering this
// round of feedback exists to fix.
// NOTE: not parallel -- captureStdout redirects the process-wide os.Stdout.
func TestReportWatchBuildErr_PhaseFailure(t *testing.T) {
	a := &app.App{Name: "my-app"}
	err := &buildPhaseFailedError{phase: "Deploy", at: "Sep 24, 2026 10:15:00 MDT"}

	out := captureStdout(t, func() {
		reportWatchBuildErr(a, err)
	})

	failureIdx := strings.Index(out, "Deploy failed at Sep 24, 2026 10:15:00 MDT")
	if failureIdx < 0 {
		t.Fatalf("expected failure line in output, got %q", out)
	}

	hintIdx := strings.Index(out, "To investigate, run: apppack -a my-app diagnose")
	if hintIdx < 0 {
		t.Fatalf("expected diagnose hint in output, got %q", out)
	}

	if hintIdx < failureIdx {
		t.Errorf("expected diagnose hint AFTER the failure line, got failure at %d, hint at %d:\n%s", failureIdx, hintIdx, out)
	}
}

// TestReportWatchBuildErr_NonPhaseFailure verifies a plain AWS/CLI error does
// not get the diagnose hint appended.
// NOTE: not parallel -- captureStdout redirects the process-wide os.Stdout.
func TestReportWatchBuildErr_NonPhaseFailure(t *testing.T) {
	a := &app.App{Name: "my-app"}
	err := errors.New("connection refused")

	out := captureStdout(t, func() {
		reportWatchBuildErr(a, err)
	})

	if !strings.Contains(out, "connection refused") {
		t.Errorf("expected the underlying error in output, got %q", out)
	}

	if strings.Contains(out, "diagnose") {
		t.Errorf("expected no diagnose hint for a non-phase-failure error, got %q", out)
	}
}
