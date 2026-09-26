package bridge_test

import (
	"reflect"
	"testing"

	"github.com/apppackio/apppack/bridge"
)

// SortInstanceClasses decides the order of the instance-class pickers in
// `create database` and `create redis`, so "smallest first" is the whole
// point of it.
//
// The only inputs are RDS DBInstanceClass values (db.*) and ElastiCache
// CacheNodeType values (cache.*), so every case here uses real ones.
func TestSortInstanceClasses(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input []string
		want  []string
	}{
		{
			name:  "rds named sizes, smallest first",
			input: []string{"db.t3.large", "db.t3.medium", "db.t3.micro", "db.t3.small"},
			want:  []string{"db.t3.micro", "db.t3.small", "db.t3.medium", "db.t3.large"},
		},
		{
			name:  "elasticache named sizes, smallest first",
			input: []string{"cache.t4g.medium", "cache.t4g.micro", "cache.t4g.small"},
			want:  []string{"cache.t4g.micro", "cache.t4g.small", "cache.t4g.medium"},
		},
		{
			// The multiplied sizes have to land above plain xlarge, and sort
			// numerically among themselves rather than as strings -- otherwise
			// db.m5.12xlarge comes before db.m5.2xlarge.
			name:  "rds multiplied sizes",
			input: []string{"db.m5.12xlarge", "db.m5.2xlarge", "db.m5.xlarge", "db.m5.24xlarge", "db.m5.4xlarge", "db.m5.large"},
			want:  []string{"db.m5.large", "db.m5.xlarge", "db.m5.2xlarge", "db.m5.4xlarge", "db.m5.12xlarge", "db.m5.24xlarge"},
		},
		{
			// The db./cache. prefix is stripped, so entries group by instance
			// family. Note what that means for the picker: a large m5 is
			// listed before a small t4g.
			name:  "groups by family, not by size across families",
			input: []string{"cache.t4g.micro", "cache.r6g.large", "cache.m6g.xlarge", "cache.m6g.large"},
			want:  []string{"cache.m6g.large", "cache.m6g.xlarge", "cache.r6g.large", "cache.t4g.micro"},
		},
		{
			name:  "mixed families and multiplied sizes",
			input: []string{"db.r5.2xlarge", "db.m5.large", "db.r5.large", "db.m5.4xlarge"},
			want:  []string{"db.m5.large", "db.m5.4xlarge", "db.r5.large", "db.r5.2xlarge"},
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

// TestSortInstanceClassesUnreachableInputs covers the two branches no current
// caller can reach: a name with no db./cache. prefix, and the "metal" weight
// in classOrder. Neither RDS nor ElastiCache offers a metal class, and both
// callers pass three-part names straight from the AWS API.
//
// Pinned rather than deleted so that removing either branch is a deliberate
// choice with a visible test change, not a silent one.
func TestSortInstanceClassesUnreachableInputs(t *testing.T) {
	t.Parallel()

	t.Run("two-part name", func(t *testing.T) {
		t.Parallel()

		got := []string{"m5.large", "m5.micro"}
		bridge.SortInstanceClasses(got)

		if want := []string{"m5.micro", "m5.large"}; !reflect.DeepEqual(got, want) {
			t.Errorf("got %v, want %v", got, want)
		}
	})

	t.Run("metal sorts last", func(t *testing.T) {
		t.Parallel()

		got := []string{"m5.metal", "m5.24xlarge", "m5.large"}
		bridge.SortInstanceClasses(got)

		if want := []string{"m5.large", "m5.24xlarge", "m5.metal"}; !reflect.DeepEqual(got, want) {
			t.Errorf("got %v, want %v", got, want)
		}
	})
}
