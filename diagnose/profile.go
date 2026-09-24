package diagnose

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/bedrock"
	bedrocktypes "github.com/aws/aws-sdk-go-v2/service/bedrock/types"
	"github.com/sirupsen/logrus"
)

// ProfileLister is the Bedrock control-plane call used to discover inference
// profiles, extracted so selection is testable without AWS.
type ProfileLister interface {
	ListInferenceProfiles(
		ctx context.Context,
		params *bedrock.ListInferenceProfilesInput,
		optFns ...func(*bedrock.Options),
	) (*bedrock.ListInferenceProfilesOutput, error)
}

// GeographyPrefixes returns the inference profile ID prefixes that keep
// inference inside a geography, in preference order.
//
// These are plural because AWS assigns prefixes per model, not per geography:
// Asia-Pacific has been served by "apac." on some models and "au." or "jp." on
// others, and Claude Sonnet 5 has no "apac." profile at all.
//
// "global." is deliberately absent. Global cross-Region inference routes to
// all commercial regions with no residency constraint, which is the exact
// property geography matching exists to prevent.
func GeographyPrefixes(g Geography) []string {
	switch g {
	case GeographyUS:
		return []string{"us."}
	case GeographyEU:
		return []string{"eu."}
	case GeographyAPAC:
		return []string{"apac.", "au.", "jp."}
	default:
		return nil
	}
}

// SelectProfile finds the active, system-defined inference profile for
// modelID whose prefix keeps inference inside geo.
func SelectProfile(ctx context.Context, l ProfileLister, geo Geography, modelID string) (string, error) {
	prefixes := GeographyPrefixes(geo)
	if len(prefixes) == 0 {
		return "", fmt.Errorf("no inference profile geography is defined for %q", geo)
	}

	found := map[string]string{}

	var nextToken *string

	for {
		out, err := l.ListInferenceProfiles(ctx, &bedrock.ListInferenceProfilesInput{
			NextToken:  nextToken,
			TypeEquals: bedrocktypes.InferenceProfileTypeSystemDefined,
		})
		if err != nil {
			return "", err
		}

		for i := range out.InferenceProfileSummaries {
			s := out.InferenceProfileSummaries[i]
			if s.Status != bedrocktypes.InferenceProfileStatusActive {
				continue
			}

			id := aws.ToString(s.InferenceProfileId)

			for _, prefix := range prefixes {
				// The profile ID is exactly prefix + modelID. A suffix check
				// alone would also match a different, longer model ID.
				if id == prefix+modelID {
					found[prefix] = id
				}
			}
		}

		if out.NextToken == nil {
			break
		}

		nextToken = out.NextToken
	}

	for _, prefix := range prefixes {
		if id, ok := found[prefix]; ok {
			logrus.WithFields(logrus.Fields{"profile": id, "geography": geo}).Debug("selected inference profile")

			return id, nil
		}
	}

	return "", fmt.Errorf(
		"no Amazon Bedrock inference profile for %s is available in the %s geography\n\n"+
			"This usually means model access is not enabled for your account, or the model\n"+
			"is not offered in your app's region. Enable it in the Bedrock console under\n"+
			"\"Model access\", or pass an explicit profile ID with --model",
		modelID, geo,
	)
}
