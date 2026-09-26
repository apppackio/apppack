// Package diagnose gathers read-only evidence about an AppPack app and asks a
// model on Amazon Bedrock to diagnose a deployment failure.
//
// Every AWS call made by this package is read-only. See
// docs/superpowers/specs/2026-09-24-diagnose-deployment-failures-design.md.
package diagnose

import (
	"fmt"
	"strings"
)

// Geography is the Bedrock cross-region inference profile geography that
// serves a given AWS region. It becomes the prefix on the model ID, e.g.
// "us" produces "us.anthropic.…".
type Geography string

const (
	GeographyUS   Geography = "us"
	GeographyEU   Geography = "eu"
	GeographyAPAC Geography = "apac"
)

// GeographyForRegion maps an AWS region to its Bedrock inference profile
// geography.
//
// Unsupported regions return an error rather than falling back to another
// geography: routing an EU app's logs to a US region would silently move
// customer data across a boundary the CLI promises not to cross.
func GeographyForRegion(region string) (Geography, error) {
	// GovCloud regions share the "us-" prefix but are a separate partition
	// with different Bedrock availability, so check them first.
	if strings.HasPrefix(region, "us-gov-") {
		return "", unsupportedRegionErr(region)
	}

	switch {
	case strings.HasPrefix(region, "us-"):
		return GeographyUS, nil
	case strings.HasPrefix(region, "eu-"):
		return GeographyEU, nil
	case strings.HasPrefix(region, "ap-"):
		return GeographyAPAC, nil
	default:
		return "", unsupportedRegionErr(region)
	}
}

func unsupportedRegionErr(region string) error {
	return fmt.Errorf(
		"`apppack diagnose` is not available for apps in %q: Amazon Bedrock inference profiles are only supported in us-*, eu-*, and ap-* regions",
		region,
	)
}
