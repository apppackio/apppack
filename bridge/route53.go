package bridge

import (
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/route53"
)

// isHostedZoneForDomain verifies that the dnsName would be a valid record in the hosted zone
func IsHostedZoneForDomain(dnsName string, hostedZone *route53.HostedZone) bool {
	return strings.HasSuffix(dnsName, strings.TrimSuffix(*hostedZone.Name, "."))
}

// HostedZoneForDomain searches AWS Hosted Zones for a place for this domain
func HostedZoneForDomain(sess *session.Session, dnsName string) (*route53.HostedZone, error) {
	r53Svc := route53.New(sess)
	nameParts := strings.Split(dnsName, ".")
	// keep stripping off subdomains until a match is found
	for i := range nameParts {
		input := route53.ListHostedZonesByNameInput{
			DNSName: aws.String(strings.Join(nameParts[i:], ".")),
		}
		resp, err := r53Svc.ListHostedZonesByName(&input)
		if err != nil {
			return nil, err
		}
		for _, zone := range resp.HostedZones {
			if IsHostedZoneForDomain(dnsName, zone) && !*zone.Config.PrivateZone {
				return zone, nil
			}
		}
	}
	return nil, fmt.Errorf("no hosted zones found for %s", dnsName)
}
