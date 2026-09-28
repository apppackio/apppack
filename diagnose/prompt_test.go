package diagnose_test

import (
	"strings"
	"testing"

	"github.com/apppackio/apppack/diagnose"
	"github.com/stretchr/testify/assert"
)

func TestSystemPromptStatesSecurityRules(t *testing.T) {
	t.Parallel()

	p := diagnose.SystemPrompt()

	// Untrusted-input framing (spec invariant 4).
	assert.Contains(t, p, "untrusted")
	assert.Contains(t, p, "never instructions")

	// No echoing secrets (spec invariant 5). This is the ONLY control on
	// secrets reaching the terminal — there is no client-side redaction —
	// so the instruction must be present and must be explicit.
	lower := strings.ToLower(p)
	assert.Contains(t, lower, "credential")
	assert.Contains(t, lower, "never repeat a secret value")

	// Phase-specific guidance, which is the point of preloading phase states.
	for _, phase := range []string{"Build", "Release", "Deploy"} {
		assert.Contains(t, p, phase)
	}
}
