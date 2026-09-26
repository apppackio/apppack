package bridge_test

import (
	"reflect"
	"testing"

	"github.com/apppackio/apppack/bridge"
)

// SortInstanceClasses decides the order of the instance-class pickers in
// `create database` and `create redis`, so "smallest first" is the whole
// point of it.
func TestSortInstanceClasses(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input []string
		want  []string
	}{
		{
			name:  "named sizes, smallest first",
			input: []string{"db.t3.large", "db.t3.nano", "db.t3.medium", "db.t3.micro", "db.t3.small"},
			want:  []string{"db.t3.nano", "db.t3.micro", "db.t3.small", "db.t3.medium", "db.t3.large"},
		},
		{
			// The multiplied sizes have to land above plain xlarge, and sort
			// numerically among themselves rather than as strings -- otherwise
			// 12xlarge comes before 2xlarge.
			name:  "multiplied sizes",
			input: []string{"db.m5.12xlarge", "db.m5.2xlarge", "db.m5.xlarge", "db.m5.4xlarge", "db.m5.large"},
			want:  []string{"db.m5.large", "db.m5.xlarge", "db.m5.2xlarge", "db.m5.4xlarge", "db.m5.12xlarge"},
		},
		{
			name:  "metal sorts last",
			input: []string{"m5.metal", "m5.24xlarge", "m5.large"},
			want:  []string{"m5.large", "m5.24xlarge", "m5.metal"},
		},
		{
			// The db./cache. prefix is stripped, so entries group by instance
			// family rather than by prefix.
			name:  "groups by family",
			input: []string{"cache.t3.micro", "cache.m5.large", "cache.m5.micro"},
			want:  []string{"cache.m5.micro", "cache.m5.large", "cache.t3.micro"},
		},
		{
			name:  "two-part names without a prefix",
			input: []string{"t3.small", "t3.micro", "t3.nano"},
			want:  []string{"t3.nano", "t3.micro", "t3.small"},
		},
		{
			name:  "already sorted",
			input: []string{"db.t3.micro", "db.t3.small"},
			want:  []string{"db.t3.micro", "db.t3.small"},
		},
		{
			name:  "empty",
			input: []string{},
			want:  []string{},
		},
		{
			name:  "single element",
			input: []string{"db.t3.micro"},
			want:  []string{"db.t3.micro"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := make([]string, len(tt.input))
			copy(got, tt.input)
			bridge.SortInstanceClasses(got)

			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("SortInstanceClasses(%v)\n got %v\nwant %v", tt.input, got, tt.want)
			}
		})
	}
}
