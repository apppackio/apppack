package diagnose_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/apppackio/apppack/diagnose"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Spec invariant 1: the tool surface is an allowlist. If this test fails
// because you added a tool, confirm the new tool is read-only, then update it.
func TestRegistryAllowlist(t *testing.T) {
	t.Parallel()

	r := diagnose.NewRegistry(diagnose.BuildTools(diagnose.ToolDeps{
		Services: []string{"web"},
	}))

	assert.ElementsMatch(t, []string{
		"get_phase_log",
		"get_app_logs",
		"get_ecs_events",
		"describe_tasks",
		"get_task_definition",
	}, r.Names())
}

// Review Focus 3: the model may name a tool that does not exist, either by
// hallucination or because injected log text told it to.
func TestRegistryRejectsUnknownTool(t *testing.T) {
	t.Parallel()

	r := diagnose.NewRegistry(nil)

	_, err := r.Call("delete_everything", map[string]any{})
	require.ErrorIs(t, err, diagnose.ErrUnknownTool)
}

func TestValidateChoice(t *testing.T) {
	t.Parallel()

	allowed := []string{"web", "worker"}

	got, err := diagnose.ValidateChoice(map[string]any{"service": "web"}, "service", allowed)
	require.NoError(t, err)
	assert.Equal(t, "web", got)

	_, err = diagnose.ValidateChoice(map[string]any{"service": "../../etc/passwd"}, "service", allowed)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be one of")
}

// Review Focus 1: an app with no successful release has no services, so every
// service name must be rejected cleanly.
func TestValidateChoiceWithNoAllowedValues(t *testing.T) {
	t.Parallel()

	_, err := diagnose.ValidateChoice(map[string]any{"service": "web"}, "service", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no services")
}

// Review Focus 4: the model may send malformed arguments.
func TestValidateChoiceMalformed(t *testing.T) {
	t.Parallel()

	allowed := []string{"web"}

	for name, args := range map[string]map[string]any{
		"missing key": {},
		"wrong type":  {"service": 42},
		"nil value":   {"service": nil},
		"nested":      {"service": map[string]any{"name": "web"}},
		"list":        {"service": []any{"web"}},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			_, err := diagnose.ValidateChoice(args, "service", allowed)
			require.Error(t, err)
		})
	}
}

func TestValidateInt(t *testing.T) {
	t.Parallel()

	// JSON numbers arrive as float64.
	got, err := diagnose.ValidateInt(map[string]any{"limit": float64(50)}, "limit", 100, 1, 500)
	require.NoError(t, err)
	assert.Equal(t, 50, got)

	// Missing key falls back to the default.
	got, err = diagnose.ValidateInt(map[string]any{}, "limit", 100, 1, 500)
	require.NoError(t, err)
	assert.Equal(t, 100, got)

	// Out of range is clamped to the cap, not rejected: the model asking for
	// too much is not a reason to fail the diagnosis.
	got, err = diagnose.ValidateInt(map[string]any{"limit": float64(99999)}, "limit", 100, 1, 500)
	require.NoError(t, err)
	assert.Equal(t, 500, got)

	// A non-numeric value is an error.
	_, err = diagnose.ValidateInt(map[string]any{"limit": "lots"}, "limit", 100, 1, 500)
	require.Error(t, err)
}

func TestTruncate(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "abc", diagnose.Truncate("abc", 10))

	out := diagnose.Truncate("abcdefghij", 5)
	assert.Contains(t, out, "truncated")
	assert.Less(t, len(out), 200)
}

// callRecorder captures which ToolDeps function was called and with what
// arguments, so tests can assert both that validation runs BEFORE a dep is
// reached (invalid input: recorder stays untouched) and that the exact
// validated value reaches the dep (valid input: recorder matches).
type callRecorder struct {
	phaseLogCalled bool
	phaseLogArg    string

	appLogsCalled  bool
	appLogsService string
	appLogsSince   int
	appLogsLimit   int

	ecsEventsCalled bool
	ecsEventsArg    string

	describeTasksCalled bool
	describeTasksArg    string

	taskDefCalled bool
	taskDefArg    string
}

func newRecorderDeps(services []string) (*callRecorder, diagnose.ToolDeps) {
	rec := &callRecorder{}

	deps := diagnose.ToolDeps{
		Services: services,
		PhaseLog: func(phase string) (string, error) {
			rec.phaseLogCalled = true
			rec.phaseLogArg = phase

			return "phase-log-output", nil
		},
		AppLogs: func(service string, sinceMinutes, limit int) (string, error) {
			rec.appLogsCalled = true
			rec.appLogsService = service
			rec.appLogsSince = sinceMinutes
			rec.appLogsLimit = limit

			return "app-logs-output", nil
		},
		ECSEvents: func(service string) (string, error) {
			rec.ecsEventsCalled = true
			rec.ecsEventsArg = service

			return "ecs-events-output", nil
		},
		DescribeTasks: func(service string) (string, error) {
			rec.describeTasksCalled = true
			rec.describeTasksArg = service

			return "describe-tasks-output", nil
		},
		TaskDefinition: func(service string) (string, error) {
			rec.taskDefCalled = true
			rec.taskDefArg = service

			return "task-def-output", nil
		},
	}

	return rec, deps
}

// TestBuildToolsValidatesBeforeInvokingDeps is the test for the task's
// central invariant: every tool's Invoke validates its arguments BEFORE
// calling its dependency, and calls the dependency with the validated value
// (not the raw argument). Deleting the ValidateChoice call from any one
// tool's Invoke must fail this test.
func TestBuildToolsValidatesBeforeInvokingDeps(t *testing.T) {
	t.Parallel()

	services := []string{"web", "worker"}

	tests := []struct {
		name        string
		toolName    string
		validArgs   map[string]any
		invalidArgs map[string]any
		wantArg     string
		wasCalled   func(*callRecorder) bool
		calledArg   func(*callRecorder) string
	}{
		{
			name:        "get_phase_log",
			toolName:    "get_phase_log",
			validArgs:   map[string]any{"phase": "build"},
			invalidArgs: map[string]any{"phase": "not-a-real-phase"},
			wantArg:     "build",
			wasCalled:   func(r *callRecorder) bool { return r.phaseLogCalled },
			calledArg:   func(r *callRecorder) string { return r.phaseLogArg },
		},
		{
			name:        "get_app_logs",
			toolName:    "get_app_logs",
			validArgs:   map[string]any{"service": "web"},
			invalidArgs: map[string]any{"service": "does-not-exist"},
			wantArg:     "web",
			wasCalled:   func(r *callRecorder) bool { return r.appLogsCalled },
			calledArg:   func(r *callRecorder) string { return r.appLogsService },
		},
		{
			name:        "get_ecs_events",
			toolName:    "get_ecs_events",
			validArgs:   map[string]any{"service": "worker"},
			invalidArgs: map[string]any{"service": "does-not-exist"},
			wantArg:     "worker",
			wasCalled:   func(r *callRecorder) bool { return r.ecsEventsCalled },
			calledArg:   func(r *callRecorder) string { return r.ecsEventsArg },
		},
		{
			name:        "describe_tasks",
			toolName:    "describe_tasks",
			validArgs:   map[string]any{"service": "web"},
			invalidArgs: map[string]any{"service": "does-not-exist"},
			wantArg:     "web",
			wasCalled:   func(r *callRecorder) bool { return r.describeTasksCalled },
			calledArg:   func(r *callRecorder) string { return r.describeTasksArg },
		},
		{
			name:        "get_task_definition",
			toolName:    "get_task_definition",
			validArgs:   map[string]any{"service": "worker"},
			invalidArgs: map[string]any{"service": "does-not-exist"},
			wantArg:     "worker",
			wasCalled:   func(r *callRecorder) bool { return r.taskDefCalled },
			calledArg:   func(r *callRecorder) string { return r.taskDefArg },
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			t.Run("invalid argument never reaches the dependency", func(t *testing.T) {
				t.Parallel()

				rec, deps := newRecorderDeps(services)
				r := diagnose.NewRegistry(diagnose.BuildTools(deps))

				_, err := r.Call(tc.toolName, tc.invalidArgs)
				require.Error(t, err)
				assert.False(t, tc.wasCalled(rec), "dependency must not be called when validation rejects the argument")
			})

			t.Run("valid argument reaches the dependency with the validated value", func(t *testing.T) {
				t.Parallel()

				rec, deps := newRecorderDeps(services)
				r := diagnose.NewRegistry(diagnose.BuildTools(deps))

				out, err := r.Call(tc.toolName, tc.validArgs)
				require.NoError(t, err)
				assert.NotEmpty(t, out)
				assert.True(t, tc.wasCalled(rec), "dependency must be called for a valid argument")
				assert.Equal(t, tc.wantArg, tc.calledArg(rec))
			})
		})
	}
}

// TestGetAppLogsPassesValidatedLimits covers get_app_logs's extra integer
// arguments: the exact clamped/defaulted values must reach AppLogs, not the
// raw request values.
func TestGetAppLogsPassesValidatedLimits(t *testing.T) {
	t.Parallel()

	rec, deps := newRecorderDeps([]string{"web"})
	r := diagnose.NewRegistry(diagnose.BuildTools(deps))

	_, err := r.Call("get_app_logs", map[string]any{
		"service":       "web",
		"since_minutes": float64(999999),
		"limit":         float64(5),
	})
	require.NoError(t, err)
	assert.True(t, rec.appLogsCalled)
	assert.Equal(t, "web", rec.appLogsService)
	assert.Equal(t, 10080, rec.appLogsSince) // clamped to the max
	assert.Equal(t, 5, rec.appLogsLimit)
}

// TestRegistryCallSuccessPath exercises Registry.Call's success path
// directly (BuildToolsValidatesBeforeInvokingDeps covers it too, but this
// pins down the return value contract in isolation).
func TestRegistryCallSuccessPath(t *testing.T) {
	t.Parallel()

	_, deps := newRecorderDeps([]string{"web"})
	r := diagnose.NewRegistry(diagnose.BuildTools(deps))

	out, err := r.Call("get_ecs_events", map[string]any{"service": "web"})
	require.NoError(t, err)
	assert.Equal(t, "ecs-events-output", out)
}

// TestRegistryCallTruncatesOversizedResult verifies Registry.Call caps a
// successful tool result at MaxToolResultBytes, keeping the tail.
func TestRegistryCallTruncatesOversizedResult(t *testing.T) {
	t.Parallel()

	big := strings.Repeat("x", diagnose.MaxToolResultBytes+100)

	deps := diagnose.ToolDeps{
		Services: []string{"web"},
		ECSEvents: func(service string) (string, error) {
			return big, nil
		},
	}

	r := diagnose.NewRegistry(diagnose.BuildTools(deps))

	out, err := r.Call("get_ecs_events", map[string]any{"service": "web"})
	require.NoError(t, err)
	assert.Less(t, len(out), len(big))
	assert.Contains(t, out, "truncated")
}

// TestRegistryCallTruncatesOversizedError verifies the error path is capped
// the same way as the success path: MaxToolResultBytes bounds cost
// regardless of whether the dependency returned data or an error.
func TestRegistryCallTruncatesOversizedError(t *testing.T) {
	t.Parallel()

	bigErr := errors.New(strings.Repeat("e", diagnose.MaxToolResultBytes+100))

	deps := diagnose.ToolDeps{
		Services: []string{"web"},
		ECSEvents: func(service string) (string, error) {
			return "", bigErr
		},
	}

	r := diagnose.NewRegistry(diagnose.BuildTools(deps))

	_, err := r.Call("get_ecs_events", map[string]any{"service": "web"})
	require.Error(t, err)
	assert.Less(t, len(err.Error()), len(bigErr.Error()))
	assert.Contains(t, err.Error(), "truncated")
}
