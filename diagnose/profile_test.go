package diagnose_test

import (
	"context"
	"testing"

	"github.com/apppackio/apppack/diagnose"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/bedrock"
	bedrocktypes "github.com/aws/aws-sdk-go-v2/service/bedrock/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeLister struct {
	pages []*bedrock.ListInferenceProfilesOutput
	calls int
	err   error
}

func (f *fakeLister) ListInferenceProfiles(
	_ context.Context, _ *bedrock.ListInferenceProfilesInput, _ ...func(*bedrock.Options),
) (*bedrock.ListInferenceProfilesOutput, error) {
	if f.err != nil {
		return nil, f.err
	}

	out := f.pages[f.calls]
	f.calls++

	return out, nil
}

func profile(id string, status bedrocktypes.InferenceProfileStatus) bedrocktypes.InferenceProfileSummary {
	return bedrocktypes.InferenceProfileSummary{
		InferenceProfileId: aws.String(id),
		Status:             status,
	}
}

func page(next *string, ids ...bedrocktypes.InferenceProfileSummary) *bedrock.ListInferenceProfilesOutput {
	return &bedrock.ListInferenceProfilesOutput{
		InferenceProfileSummaries: ids,
		NextToken:                 next,
	}
}

func TestGeographyPrefixes(t *testing.T) {
	t.Parallel()

	assert.Equal(t, []string{"us."}, diagnose.GeographyPrefixes(diagnose.GeographyUS))
	assert.Equal(t, []string{"eu."}, diagnose.GeographyPrefixes(diagnose.GeographyEU))

	// Asia-Pacific has been served by different prefixes on different models,
	// so all of them are acceptable.
	assert.Equal(t, []string{"apac.", "au.", "jp."}, diagnose.GeographyPrefixes(diagnose.GeographyAPAC))
}

// "global." routes to all commercial regions with no residency constraint,
// which is exactly what geography matching exists to prevent.
func TestGeographyPrefixesNeverIncludesGlobal(t *testing.T) {
	t.Parallel()

	for _, g := range []diagnose.Geography{diagnose.GeographyUS, diagnose.GeographyEU, diagnose.GeographyAPAC} {
		assert.NotContains(t, diagnose.GeographyPrefixes(g), "global.")
	}
}

func TestSelectProfile(t *testing.T) {
	t.Parallel()

	l := &fakeLister{pages: []*bedrock.ListInferenceProfilesOutput{
		page(nil,
			profile("eu.anthropic.claude-sonnet-5", bedrocktypes.InferenceProfileStatusActive),
			profile("us.anthropic.claude-sonnet-5", bedrocktypes.InferenceProfileStatusActive),
			profile("global.anthropic.claude-sonnet-5", bedrocktypes.InferenceProfileStatusActive),
		),
	}}

	got, err := diagnose.SelectProfile(context.Background(), l, diagnose.GeographyUS, "anthropic.claude-sonnet-5")
	require.NoError(t, err)
	assert.Equal(t, "us.anthropic.claude-sonnet-5", got)
}

// The APAC case the hardcoded map got wrong: no apac. profile exists, but au.
// does, and it keeps inference inside the geography.
func TestSelectProfileFallsToAnyInGeographyPrefix(t *testing.T) {
	t.Parallel()

	l := &fakeLister{pages: []*bedrock.ListInferenceProfilesOutput{
		page(nil,
			profile("us.anthropic.claude-sonnet-5", bedrocktypes.InferenceProfileStatusActive),
			profile("au.anthropic.claude-sonnet-5", bedrocktypes.InferenceProfileStatusActive),
		),
	}}

	got, err := diagnose.SelectProfile(context.Background(), l, diagnose.GeographyAPAC, "anthropic.claude-sonnet-5")
	require.NoError(t, err)
	assert.Equal(t, "au.anthropic.claude-sonnet-5", got)
}

// A global profile must never be selected, even when it is the only one for
// the model.
func TestSelectProfileRejectsGlobal(t *testing.T) {
	t.Parallel()

	l := &fakeLister{pages: []*bedrock.ListInferenceProfilesOutput{
		page(nil, profile("global.anthropic.claude-sonnet-5", bedrocktypes.InferenceProfileStatusActive)),
	}}

	_, err := diagnose.SelectProfile(context.Background(), l, diagnose.GeographyAPAC, "anthropic.claude-sonnet-5")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--model")
}

func TestSelectProfileIgnoresInactiveAndOtherModels(t *testing.T) {
	t.Parallel()

	l := &fakeLister{pages: []*bedrock.ListInferenceProfilesOutput{
		page(nil,
			profile("us.anthropic.claude-sonnet-5", bedrocktypes.InferenceProfileStatus("INACTIVE")),
			profile("us.anthropic.claude-haiku-4-5", bedrocktypes.InferenceProfileStatusActive),
		),
	}}

	_, err := diagnose.SelectProfile(context.Background(), l, diagnose.GeographyUS, "anthropic.claude-sonnet-5")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "anthropic.claude-sonnet-5")
}

func TestSelectProfilePaginates(t *testing.T) {
	t.Parallel()

	l := &fakeLister{pages: []*bedrock.ListInferenceProfilesOutput{
		page(aws.String("more"), profile("us.anthropic.claude-haiku-4-5", bedrocktypes.InferenceProfileStatusActive)),
		page(nil, profile("us.anthropic.claude-sonnet-5", bedrocktypes.InferenceProfileStatusActive)),
	}}

	got, err := diagnose.SelectProfile(context.Background(), l, diagnose.GeographyUS, "anthropic.claude-sonnet-5")
	require.NoError(t, err)
	assert.Equal(t, "us.anthropic.claude-sonnet-5", got)
	assert.Equal(t, 2, l.calls)
}

// An exact-match requirement: a different model whose ID merely ends with the
// pinned one's text must not be accepted.
func TestSelectProfileDoesNotMatchLongerModelID(t *testing.T) {
	t.Parallel()

	l := &fakeLister{pages: []*bedrock.ListInferenceProfilesOutput{
		page(nil, profile("us.anthropic.claude-sonnet-5-preview", bedrocktypes.InferenceProfileStatusActive)),
	}}

	_, err := diagnose.SelectProfile(context.Background(), l, diagnose.GeographyUS, "anthropic.claude-sonnet-5")
	require.Error(t, err)
}
