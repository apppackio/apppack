package diagnose

import (
	"errors"
	"fmt"
	"strings"

	"github.com/aws/smithy-go"
)

// TranslateError converts a Bedrock API error into a message that tells the
// user what to do about it.
//
// The CLI ships before the IAM change reaches every account, so for a period
// the permission error IS this feature's front door. It leads with the stack
// upgrade because that is the expected cause during rollout; the two
// AccessDenied causes cannot be distinguished from the error alone.
//
// Matching is done on smithy.APIError's ErrorCode() rather than on the
// concrete exception types. Bedrock has two SDK packages with distinct Go
// types for the same error names: the control-plane package
// ("github.com/aws/aws-sdk-go-v2/service/bedrock", used by SelectProfile's
// ListInferenceProfiles call) and the data-plane package
// ("github.com/aws/aws-sdk-go-v2/service/bedrockruntime", used by Converse).
// A *bedrock/types.AccessDeniedException and a
// *bedrockruntime/types.AccessDeniedException do not unify via errors.As
// against a single concrete type, so a type-assertion-based version of this
// function would only translate errors from whichever package it happened to
// import -- and ListInferenceProfiles is the very first Bedrock call this
// command makes, so a customer without Bedrock IAM access would hit that gap
// on every default invocation. Matching on the shared smithy.APIError
// interface handles both packages (and any future one) uniformly.
func TranslateError(err error, appName string, pipeline bool, region string) error {
	if err == nil {
		return nil
	}

	var apiErr smithy.APIError
	if !errors.As(err, &apiErr) {
		return err
	}

	switch apiErr.ErrorCode() {
	case "AccessDeniedException":
		return fmt.Errorf(`not authorized to invoke Amazon Bedrock in %s

This usually means one of two things:

  1. Your AppPack stack predates Bedrock support. Upgrade it:

       apppack %s

  2. Model access is not enabled in your AWS account. Open the Bedrock
     console in %s, go to "Model access", and enable the model

Original error: %w`, region, upgradeCommand(appName, pipeline), region, err)

	case "ThrottlingException":
		return fmt.Errorf("request throttled by Amazon Bedrock; wait a moment and try again: %w", err)

	case "ValidationException":
		if strings.Contains(strings.ToLower(apiErr.ErrorMessage()), "tool") {
			return fmt.Errorf(`the selected model does not support tool use, which `+"`apppack diagnose`"+` requires

Pass a model that supports tool use with --model

Original error: %w`, err)
		}
	}

	return err
}

// upgradeCommand returns the stack upgrade command for this app. Pipelines and
// review apps are upgraded through `upgrade pipeline`, not `upgrade app`.
func upgradeCommand(appName string, pipeline bool) string {
	if pipeline {
		return "upgrade pipeline " + appName
	}

	return "upgrade app " + appName
}
