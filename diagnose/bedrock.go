package diagnose

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/document"
	brtypes "github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
	"github.com/sirupsen/logrus"
)

const (
	// MaxRounds caps tool-call iterations so a confused model cannot loop up
	// the customer's bill.
	MaxRounds = 12

	// MaxTotalTokens caps cumulative token usage across the run.
	MaxTotalTokens = 400000

	// maxResponseTokens caps a single response.
	maxResponseTokens = 4096

	// DefaultModelID is the Bedrock model used when --model is not given.
	// It is combined with the app's geography by ModelIDForGeography.
	DefaultModelID = "anthropic.claude-sonnet-4-5-20250929-v1:0"
)

// Converser is the subset of the Bedrock runtime client this package uses,
// extracted so the loop is testable without network access.
type Converser interface {
	Converse(
		ctx context.Context,
		params *bedrockruntime.ConverseInput,
		optFns ...func(*bedrockruntime.Options),
	) (*bedrockruntime.ConverseOutput, error)
}

// ModelIDForGeography prefixes a model ID with its cross-region inference
// profile geography. An ID that already carries a known prefix is returned
// unchanged so --model can name an exact profile.
func ModelIDForGeography(g Geography, modelID string) string {
	for _, p := range []string{"us.", "eu.", "apac."} {
		if strings.HasPrefix(modelID, p) {
			return modelID
		}
	}

	return string(g) + "." + modelID
}

// toolConfig converts the registry into a Bedrock ToolConfiguration.
func toolConfig(r *Registry) *brtypes.ToolConfiguration {
	tools := r.Tools()
	if len(tools) == 0 {
		return nil
	}

	specs := make([]brtypes.Tool, 0, len(tools))

	for _, t := range tools {
		specs = append(specs, &brtypes.ToolMemberToolSpec{
			Value: brtypes.ToolSpecification{
				Name:        aws.String(t.Name),
				Description: aws.String(t.Description),
				InputSchema: &brtypes.ToolInputSchemaMemberJson{
					Value: document.NewLazyDocument(t.Schema),
				},
			},
		})
	}

	return &brtypes.ToolConfiguration{Tools: specs}
}

// decodeToolInput converts a tool_use input document into a plain map.
// A document that is not a JSON object yields an empty map rather than a
// panic; validation in the tool then reports the missing arguments.
func decodeToolInput(d document.Interface) map[string]any {
	if d == nil {
		return map[string]any{}
	}

	var raw json.RawMessage
	if err := d.UnmarshalSmithyDocument(&raw); err != nil {
		logrus.WithError(err).Debug("could not unmarshal tool input document")

		return map[string]any{}
	}

	args := map[string]any{}
	if err := json.Unmarshal(raw, &args); err != nil {
		logrus.WithError(err).Debug("tool input was not a JSON object")

		return map[string]any{}
	}

	return args
}

// Run drives the tool loop until the model produces a text answer.
func Run(
	ctx context.Context, c Converser, modelID, system, userMessage string, r *Registry,
) (string, error) {
	messages := []brtypes.Message{{
		Role:    brtypes.ConversationRoleUser,
		Content: []brtypes.ContentBlock{&brtypes.ContentBlockMemberText{Value: userMessage}},
	}}

	totalTokens := 0

	for round := 0; round < MaxRounds; round++ {
		out, err := c.Converse(ctx, &bedrockruntime.ConverseInput{
			ModelId:  aws.String(modelID),
			Messages: messages,
			System: []brtypes.SystemContentBlock{
				&brtypes.SystemContentBlockMemberText{Value: system},
			},
			InferenceConfig: &brtypes.InferenceConfiguration{
				MaxTokens: aws.Int32(maxResponseTokens),
			},
			ToolConfig: toolConfig(r),
		})
		if err != nil {
			return "", err
		}

		if out.Usage != nil && out.Usage.TotalTokens != nil {
			totalTokens += int(*out.Usage.TotalTokens)
			if totalTokens > MaxTotalTokens {
				return "", fmt.Errorf(
					"diagnosis exceeded its token budget (%d tokens) without reaching a conclusion", MaxTotalTokens,
				)
			}
		}

		msg, ok := out.Output.(*brtypes.ConverseOutputMemberMessage)
		if !ok {
			return "", errors.New("unexpected response shape from Bedrock")
		}

		messages = append(messages, msg.Value)

		if out.StopReason != brtypes.StopReasonToolUse {
			return collectText(msg.Value.Content), nil
		}

		results := runToolCalls(msg.Value.Content, r)
		if len(results) == 0 {
			return collectText(msg.Value.Content), nil
		}

		messages = append(messages, brtypes.Message{
			Role:    brtypes.ConversationRoleUser,
			Content: results,
		})
	}

	return "", fmt.Errorf("diagnosis used its %d allowed steps without reaching a conclusion", MaxRounds)
}

// runToolCalls executes every tool_use block in a response, returning the
// matching tool_result blocks. A failure becomes an error result so the model
// can recover; it never aborts the run.
func runToolCalls(content []brtypes.ContentBlock, r *Registry) []brtypes.ContentBlock {
	var results []brtypes.ContentBlock

	for _, block := range content {
		use, ok := block.(*brtypes.ContentBlockMemberToolUse)
		if !ok {
			continue
		}

		name := aws.ToString(use.Value.Name)
		args := decodeToolInput(use.Value.Input)

		logrus.WithFields(logrus.Fields{"tool": name, "args": args}).Debug("model requested tool")

		text, err := r.Call(name, args)
		status := brtypes.ToolResultStatusSuccess

		if err != nil {
			status = brtypes.ToolResultStatusError
			text = err.Error()
		}

		results = append(results, &brtypes.ContentBlockMemberToolResult{
			Value: brtypes.ToolResultBlock{
				ToolUseId: use.Value.ToolUseId,
				Status:    status,
				Content: []brtypes.ToolResultContentBlock{
					&brtypes.ToolResultContentBlockMemberText{Value: text},
				},
			},
		})
	}

	return results
}

func collectText(content []brtypes.ContentBlock) string {
	var b strings.Builder

	for _, block := range content {
		if t, ok := block.(*brtypes.ContentBlockMemberText); ok {
			b.WriteString(t.Value)
		}
	}

	return strings.TrimSpace(b.String())
}
