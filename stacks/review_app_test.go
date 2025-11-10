package stacks_test

import (
	"testing"

	"github.com/apppackio/apppack/stacks"
)

func TestIsReviewAppName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		appName  string
		expected bool
	}{
		{
			name:     "review app with colon",
			appName:  "my-pipeline:123",
			expected: true,
		},
		{
			name:     "review app with PR number",
			appName:  "frontend:456",
			expected: true,
		},
		{
			name:     "regular app name",
			appName:  "my-app",
			expected: false,
		},
		{
			name:     "pipeline name",
			appName:  "my-pipeline",
			expected: false,
		},
		{
			name:     "app with hyphen",
			appName:  "my-app-name",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := stacks.IsReviewAppName(tt.appName)
			if result != tt.expected {
				t.Errorf("IsReviewAppName(%q) = %v, expected %v", tt.appName, result, tt.expected)
			}
		})
	}
}
