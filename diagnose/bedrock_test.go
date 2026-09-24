package diagnose_test

import (
	"context"
	"testing"

	"github.com/apppackio/apppack/diagnose"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/document"
	brtypes "github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeConverser struct {
	responses []*bedrockruntime.ConverseOutput
	calls     int
	lastInput *bedrockruntime.ConverseInput
}

func (f *fakeConverser) Converse(
	_ context.Context, in *bedrockruntime.ConverseInput, _ ...func(*bedrockruntime.Options),
) (*bedrockruntime.ConverseOutput, error) {
	f.lastInput = in
	resp := f.responses[f.calls]
	f.calls++

	return resp, nil
}

func textResponse(s string) *bedrockruntime.ConverseOutput {
	return &bedrockruntime.ConverseOutput{
		StopReason: brtypes.StopReasonEndTurn,
		Usage:      &brtypes.TokenUsage{TotalTokens: aws.Int32(100)},
		Output: &brtypes.ConverseOutputMemberMessage{
			Value: brtypes.Message{
				Role:    brtypes.ConversationRoleAssistant,
				Content: []brtypes.ContentBlock{&brtypes.ContentBlockMemberText{Value: s}},
			},
		},
	}
}

func toolUseResponse(id, name string, input any) *bedrockruntime.ConverseOutput {
	return &bedrockruntime.ConverseOutput{
		StopReason: brtypes.StopReasonToolUse,
		Usage:      &brtypes.TokenUsage{TotalTokens: aws.Int32(100)},
		Output: &brtypes.ConverseOutputMemberMessage{
			Value: brtypes.Message{
				Role: brtypes.ConversationRoleAssistant,
				Content: []brtypes.ContentBlock{
					&brtypes.ContentBlockMemberToolUse{
						Value: brtypes.ToolUseBlock{
							ToolUseId: aws.String(id),
							Name:      aws.String(name),
							Input:     document.NewLazyDocument(input),
						},
					},
				},
			},
		},
	}
}

func TestRunReturnsTextAnswer(t *testing.T) {
	t.Parallel()

	c := &fakeConverser{responses: []*bedrockruntime.ConverseOutput{
		textResponse("Your web process is not binding to $PORT."),
	}}

	out, err := diagnose.Run(context.Background(), c, "model", "system", "context", diagnose.NewRegistry(nil))
	require.NoError(t, err)
	assert.Equal(t, "Your web process is not binding to $PORT.", out)
	assert.Equal(t, 1, c.calls)
}

func TestRunExecutesTool(t *testing.T) {
	t.Parallel()

	called := false
	r := diagnose.NewRegistry([]diagnose.Tool{{
		Name:   "get_ecs_events",
		Schema: map[string]any{"type": "object"},
		Invoke: func(map[string]any) (string, error) {
			called = true

			return "health check failed", nil
		},
	}})

	c := &fakeConverser{responses: []*bedrockruntime.ConverseOutput{
		toolUseResponse("t1", "get_ecs_events", map[string]any{"service": "web"}),
		textResponse("Health checks are failing."),
	}}

	out, err := diagnose.Run(context.Background(), c, "model", "system", "context", r)
	require.NoError(t, err)
	assert.True(t, called)
	assert.Equal(t, "Health checks are failing.", out)
	assert.Equal(t, 2, c.calls)
}

// Review Focus 3: a hallucinated or injected tool name must not crash the run.
func TestRunHandlesUnknownTool(t *testing.T) {
	t.Parallel()

	c := &fakeConverser{responses: []*bedrockruntime.ConverseOutput{
		toolUseResponse("t1", "rm_rf_slash", map[string]any{}),
		textResponse("I could not use that tool."),
	}}

	out, err := diagnose.Run(context.Background(), c, "model", "system", "context", diagnose.NewRegistry(nil))
	require.NoError(t, err)
	assert.Equal(t, "I could not use that tool.", out)
	assert.Equal(t, 2, c.calls)
}

// A tool that returns an error must send an error result, not abort the run.
func TestRunHandlesToolError(t *testing.T) {
	t.Parallel()

	r := diagnose.NewRegistry([]diagnose.Tool{{
		Name:   "get_phase_log",
		Schema: map[string]any{"type": "object"},
		Invoke: func(map[string]any) (string, error) {
			return "", assert.AnError
		},
	}})

	c := &fakeConverser{responses: []*bedrockruntime.ConverseOutput{
		toolUseResponse("t1", "get_phase_log", map[string]any{"phase": "build"}),
		textResponse("The build phase has no log."),
	}}

	out, err := diagnose.Run(context.Background(), c, "model", "system", "context", r)
	require.NoError(t, err)
	assert.Equal(t, "The build phase has no log.", out)
}

func TestRunStopsAtMaxRounds(t *testing.T) {
	t.Parallel()

	r := diagnose.NewRegistry([]diagnose.Tool{{
		Name:   "get_ecs_events",
		Schema: map[string]any{"type": "object"},
		Invoke: func(map[string]any) (string, error) { return "more events", nil },
	}})

	responses := make([]*bedrockruntime.ConverseOutput, 0, diagnose.MaxRounds+2)
	for i := 0; i < diagnose.MaxRounds+2; i++ {
		responses = append(responses, toolUseResponse("t", "get_ecs_events", map[string]any{"service": "web"}))
	}

	c := &fakeConverser{responses: responses}

	_, err := diagnose.Run(context.Background(), c, "model", "system", "context", r)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "without reaching a conclusion")
	assert.LessOrEqual(t, c.calls, diagnose.MaxRounds)
}

func TestModelIDForGeography(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "us.anthropic.foo", diagnose.ModelIDForGeography(diagnose.GeographyUS, "anthropic.foo"))
	assert.Equal(t, "eu.anthropic.foo", diagnose.ModelIDForGeography(diagnose.GeographyEU, "anthropic.foo"))
	assert.Equal(t, "apac.anthropic.foo", diagnose.ModelIDForGeography(diagnose.GeographyAPAC, "anthropic.foo"))

	// An ID that already carries a geography prefix is passed through, so a
	// user can name an exact inference profile with --model.
	assert.Equal(t, "us.anthropic.foo", diagnose.ModelIDForGeography(diagnose.GeographyEU, "us.anthropic.foo"))
}
