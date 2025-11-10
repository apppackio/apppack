package bridge

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/route53"
	"github.com/aws/aws-sdk-go-v2/service/route53/types"
)

// isHostedZoneForDomain verifies that the dnsName would be a valid record in the hosted zone
func IsHostedZoneForDomain(dnsName string, hostedZone *types.HostedZone) bool {
	return strings.HasSuffix(dnsName, strings.TrimSuffix(*hostedZone.Name, "."))
}

// HostedZoneForDomain searches AWS Hosted Zones for a place for this domain
func HostedZoneForDomain(cfg aws.Config, dnsName string) (*types.HostedZone, error) {
	r53Svc := route53.NewFromConfig(cfg)
	nameParts := strings.Split(dnsName, ".")
	// keep stripping off subdomains until a match is found
	for i := range nameParts {
		dnsNameStr := strings.Join(nameParts[i:], ".")
		input := route53.ListHostedZonesByNameInput{
			DNSName: &dnsNameStr,
		}

		resp, err := r53Svc.ListHostedZonesByName(context.Background(), &input)
		if err != nil {
			return nil, err
		}

		for _, zone := range resp.HostedZones {
			if IsHostedZoneForDomain(dnsName, &zone) && !zone.Config.PrivateZone {
				return &zone, nil
			}
		}
	}

	return nil, fmt.Errorf("no hosted zones found for %s", dnsName)
}
