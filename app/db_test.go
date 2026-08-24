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
		name          string
		configVars    ConfigVariables
		wantErrSub    string
		notWantErrSub string
	}{
		{
			name:       "DATABASE_URL present -- points at modify app",
			configVars: ConfigVariables{{Name: "DATABASE_URL", Value: "postgres://example"}},
			wantErrSub: "apppack modify app myapp",
		},
		{
			// The engine is inferred from DATABASE_URL, so there is no
			// --external-database flag to advertise. Guard against a stale
			// reference creeping back into user-facing text.
			name:          "DATABASE_URL present -- does not advertise a removed flag",
			configVars:    ConfigVariables{{Name: "DATABASE_URL", Value: "postgres://example"}},
			wantErrSub:    "db utils are not enabled",
			notWantErrSub: "--external-database",
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

			if tt.notWantErrSub != "" && strings.Contains(err.Error(), tt.notWantErrSub) {
				t.Errorf("error %q should not contain %q", err.Error(), tt.notWantErrSub)
			}
		})
	}
}
