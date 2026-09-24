package diagnose_test

import (
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
