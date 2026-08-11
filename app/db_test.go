package app

import (
	"strings"
	"testing"
)

// TestNoDatabaseConfiguredErrorFromConfig covers both branches of the "no database
// configured" helper: a DATABASE_URL config variable present (external database,
// db utils not enabled) vs absent (no database at all).
func TestNoDatabaseConfiguredErrorFromConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		configVars ConfigVariables
		wantErrSub string
	}{
		{
			name:       "DATABASE_URL present -- points at modify app / --external-database",
			configVars: ConfigVariables{{Name: "DATABASE_URL", Value: "postgres://example"}},
			wantErrSub: "apppack modify app myapp",
		},
		{
			name:       "DATABASE_URL present -- mentions --external-database",
			configVars: ConfigVariables{{Name: "DATABASE_URL", Value: "postgres://example"}},
			wantErrSub: "--external-database",
		},
		{
			name:       "DATABASE_URL absent -- points at create database",
			configVars: ConfigVariables{{Name: "OTHER_VAR", Value: "foo"}},
			wantErrSub: "apppack create database",
		},
		{
			name:       "no config vars at all -- points at create database",
			configVars: nil,
			wantErrSub: "apppack create database",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := noDatabaseConfiguredErrorFromConfig("myapp", tt.configVars)
			if err == nil {
				t.Fatal("expected an error, got nil")
			}

			if !strings.Contains(err.Error(), tt.wantErrSub) {
				t.Errorf("error %q does not contain %q", err.Error(), tt.wantErrSub)
			}
		})
	}
}
