package diagnose_test

import (
	"testing"

	"github.com/apppackio/apppack/diagnose"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGeographyForRegion(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		region string
		want   diagnose.Geography
	}{
		{"us-east-1", diagnose.GeographyUS},
		{"us-west-2", diagnose.GeographyUS},
		{"eu-west-1", diagnose.GeographyEU},
		{"eu-central-1", diagnose.GeographyEU},
		{"ap-southeast-2", diagnose.GeographyAPAC},
		{"ap-northeast-1", diagnose.GeographyAPAC},
	} {
		t.Run(tc.region, func(t *testing.T) {
			t.Parallel()

			got, err := diagnose.GeographyForRegion(tc.region)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

// Review Focus 5: us-gov-* matches a naive "us-" prefix but is GovCloud.
func TestGeographyForRegionUnsupported(t *testing.T) {
	t.Parallel()

	for _, region := range []string{
		"us-gov-west-1",
		"us-gov-east-1",
		"ca-central-1",
		"sa-east-1",
		"me-south-1",
		"af-south-1",
		"",
	} {
		t.Run(region, func(t *testing.T) {
			t.Parallel()

			_, err := diagnose.GeographyForRegion(region)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "not available")
		})
	}
}
