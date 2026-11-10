package bridge_test

import (
	"testing"

	"github.com/apppackio/apppack/bridge"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/route53/types"
)

func zone(name string) *types.HostedZone {
	return &types.HostedZone{Name: aws.String(name)}
}

// TestIsHostedZoneForDomainRequiresLabelBoundary -- a zone only contains a
// name that is the zone itself or sits under one of its labels. "example.com"
// is not a place to put a record for "notexample.com".
func TestIsHostedZoneForDomainRequiresLabelBoundary(t *testing.T) {
	t.Parallel()

	if bridge.IsHostedZoneForDomain("notexample.com", zone("example.com.")) {
		t.Error("notexample.com matched the example.com zone")
	}
}

func TestIsHostedZoneForDomain(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		dnsName  string
		zoneName string
		want     bool
	}{
		{"zone apex", "example.com", "example.com.", true},
		{"subdomain", "www.example.com", "example.com.", true},
		{"nested subdomain", "a.b.example.com", "example.com.", true},
		{"wildcard", "*.example.com", "example.com.", true},
		{"trailing dot on the name", "www.example.com.", "example.com.", true},
		{"zone stored without trailing dot", "www.example.com", "example.com", true},

		{"shared suffix, different domain", "notexample.com", "example.com.", false},
		{"different tld", "example.org", "example.com.", false},
		{"name is a parent of the zone", "com", "example.com.", false},
		{"name is shorter than the zone", "example.com", "www.example.com.", false},
		{"unrelated", "example.net", "example.com.", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := bridge.IsHostedZoneForDomain(tt.dnsName, zone(tt.zoneName))
			if got != tt.want {
				t.Errorf("IsHostedZoneForDomain(%q, %q) = %v, want %v",
					tt.dnsName, tt.zoneName, got, tt.want)
			}
		})
	}
}
