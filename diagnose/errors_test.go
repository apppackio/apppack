package diagnose_test

import (
	"errors"
	"testing"

	"github.com/apppackio/apppack/diagnose"
	"github.com/aws/aws-sdk-go-v2/aws"
	bedrocktypes "github.com/aws/aws-sdk-go-v2/service/bedrock/types"
	brtypes "github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
	"github.com/aws/smithy-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTranslateErrorAccessDeniedApp(t *testing.T) {
	t.Parallel()

	err := diagnose.TranslateError(
		&brtypes.AccessDeniedException{Message: aws.String("not authorized")},
		"myapp", false, "us-east-1",
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "apppack upgrade app myapp")
	assert.Contains(t, err.Error(), "Model access")
	assert.Contains(t, err.Error(), "us-east-1")
}

func TestTranslateErrorAccessDeniedPipeline(t *testing.T) {
	t.Parallel()

	err := diagnose.TranslateError(
		&brtypes.AccessDeniedException{Message: aws.String("not authorized")},
		"mypipeline", true, "eu-west-1",
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "apppack upgrade pipeline mypipeline")
	assert.NotContains(t, err.Error(), "upgrade app ")
}

func TestTranslateErrorAccessDeniedControlPlane(t *testing.T) {
	t.Parallel()

	// SelectProfile calls bedrock.ListInferenceProfiles, a control-plane API
	// whose AccessDeniedException is a distinct Go type from the data-plane
	// bedrockruntime one used elsewhere in this file. This is the error
	// TranslateError must also recognize: it is the first Bedrock call
	// `apppack diagnose` makes, so on an account without Bedrock IAM access
	// this is the error the default invocation actually hits.
	err := diagnose.TranslateError(
		&bedrocktypes.AccessDeniedException{Message: aws.String("not authorized")},
		"myapp", false, "us-east-1",
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "apppack upgrade app myapp")
	assert.Contains(t, err.Error(), "Model access")
	assert.Contains(t, err.Error(), "us-east-1")
}

func TestTranslateErrorThrottlingControlPlane(t *testing.T) {
	t.Parallel()

	err := diagnose.TranslateError(
		&bedrocktypes.ThrottlingException{Message: aws.String("slow down")},
		"myapp", false, "us-east-1",
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "throttled")
}

func TestTranslateErrorValidationMentionsToolSupport(t *testing.T) {
	t.Parallel()

	err := diagnose.TranslateError(
		&smithy.GenericAPIError{Code: "ValidationException", Message: "This model doesn't support tool use"},
		"myapp", false, "us-east-1",
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--model")
	assert.Contains(t, err.Error(), "tool use")
}

func TestTranslateErrorThrottling(t *testing.T) {
	t.Parallel()

	err := diagnose.TranslateError(
		&brtypes.ThrottlingException{Message: aws.String("slow down")},
		"myapp", false, "us-east-1",
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "throttled")
}

func TestTranslateErrorPassesThroughUnknown(t *testing.T) {
	t.Parallel()

	orig := errors.New("something else")
	assert.Equal(t, orig, diagnose.TranslateError(orig, "myapp", false, "us-east-1"))
}

func TestTranslateErrorNil(t *testing.T) {
	t.Parallel()

	assert.NoError(t, diagnose.TranslateError(nil, "myapp", false, "us-east-1"))
}
