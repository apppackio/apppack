package app_test

import (
	"testing"

	"github.com/apppackio/apppack/app"
	"github.com/aws/aws-sdk-go-v2/aws"
)

func TestDetermineBuildSourceVersion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		isReviewApp     bool
		reviewAppNumber *string
		ref             string
		expected        *string
	}{
		{
			name:            "review app with PR number",
			isReviewApp:     true,
			reviewAppNumber: aws.String("123"),
			ref:             "",
			expected:        aws.String("pr/123"),
		},
		{
			name:            "review app without PR number",
			isReviewApp:     true,
			reviewAppNumber: nil,
			ref:             "",
			expected:        nil,
		},
		{
			name:        "regular app with branch ref",
			isReviewApp: false,
			ref:         "develop",
			expected:    aws.String("develop"),
		},
		{
			name:        "regular app with tag ref",
			isReviewApp: false,
			ref:         "v1.2.3",
			expected:    aws.String("v1.2.3"),
		},
		{
			name:        "regular app with commit hash",
			isReviewApp: false,
			ref:         "abc123def456",
			expected:    aws.String("abc123def456"),
		},
		{
			name:        "regular app without ref",
			isReviewApp: false,
			ref:         "",
			expected:    nil,
		},
		{
			name:            "review app takes precedence over ref",
			isReviewApp:     true,
			reviewAppNumber: aws.String("456"),
			ref:             "main",
			expected:        aws.String("pr/456"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := app.DetermineBuildSourceVersion(tt.isReviewApp, tt.reviewAppNumber, tt.ref)

			if tt.expected == nil {
				if result != nil {
					t.Errorf("DetermineBuildSourceVersion() = %v, expected nil", *result)
				}
			} else if result == nil {
				t.Errorf("DetermineBuildSourceVersion() = nil, expected %v", *tt.expected)
			} else if *result != *tt.expected {
				t.Errorf("DetermineBuildSourceVersion() = %v, expected %v", *result, *tt.expected)
			}
		})
	}
}
