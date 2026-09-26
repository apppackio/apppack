package stringslice_test

import (
	"reflect"
	"testing"

	"github.com/apppackio/apppack/stringslice"
)

func TestContains(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		needle string
		list   []string
		want   bool
	}{
		{"present", "b", []string{"a", "b", "c"}, true},
		{"absent", "z", []string{"a", "b", "c"}, false},
		{"first", "a", []string{"a", "b"}, true},
		{"last", "b", []string{"a", "b"}, true},
		{"empty list", "a", []string{}, false},
		{"nil list", "a", nil, false},
		{"empty needle against empty entry", "", []string{""}, true},
		{"empty needle against non-empty list", "", []string{"a"}, false},
		{"case sensitive", "A", []string{"a"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := stringslice.Contains(tt.needle, tt.list); got != tt.want {
				t.Errorf("Contains(%q, %v) = %v, want %v", tt.needle, tt.list, got, tt.want)
			}
		})
	}
}

func TestDeduplicate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		input      []string
		wantResult []string
		wantDupes  []string
	}{
		{
			name:       "no duplicates preserves order",
			input:      []string{"c", "a", "b"},
			wantResult: []string{"c", "a", "b"},
			wantDupes:  nil,
		},
		{
			name:       "keeps first occurrence",
			input:      []string{"a", "b", "a"},
			wantResult: []string{"a", "b"},
			wantDupes:  []string{"a"},
		},
		{
			// `apppack access` prints the dupes back to the user, so a value
			// repeated three times has to report twice, not once.
			name:       "reports every extra occurrence",
			input:      []string{"a", "a", "a"},
			wantResult: []string{"a"},
			wantDupes:  []string{"a", "a"},
		},
		{
			name:       "multiple distinct duplicates",
			input:      []string{"a", "b", "a", "b", "c"},
			wantResult: []string{"a", "b", "c"},
			wantDupes:  []string{"a", "b"},
		},
		{
			name:       "empty input returns nil, not empty slices",
			input:      []string{},
			wantResult: nil,
			wantDupes:  nil,
		},
		{
			name:       "nil input",
			input:      nil,
			wantResult: nil,
			wantDupes:  nil,
		},
		{
			name:       "empty string is a value like any other",
			input:      []string{"", "", "a"},
			wantResult: []string{"", "a"},
			wantDupes:  []string{""},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result, dupes := stringslice.Deduplicate(tt.input)

			if !reflect.DeepEqual(result, tt.wantResult) {
				t.Errorf("result = %#v, want %#v", result, tt.wantResult)
			}

			if !reflect.DeepEqual(dupes, tt.wantDupes) {
				t.Errorf("dupes = %#v, want %#v", dupes, tt.wantDupes)
			}
		})
	}
}

// TestDeduplicateDoesNotAliasInput guards the caller pattern in cmd/access.go
// and cmd/admins.go, which assign the result back over the input slice.
func TestDeduplicateDoesNotAliasInput(t *testing.T) {
	t.Parallel()

	input := []string{"a", "b", "a"}

	result, _ := stringslice.Deduplicate(input)
	result[0] = "mutated"

	if input[0] != "a" {
		t.Errorf("writing to the result changed the input: input[0] = %q", input[0])
	}
}
