package cmd

import (
	"testing"

	"github.com/apppackio/apppack/app"
	"github.com/aws/aws-sdk-go-v2/aws"
)

// TestShouldHintEnableDBUtils covers the decision behind the follow-up hint printed
// after DATABASE_URL is stored. Settings is pre-populated in every case, which makes
// App.LoadSettings short-circuit, so none of these touch AWS.
func TestShouldHintEnableDBUtils(t *testing.T) {
	t.Parallel()

	settingsWithEngine := func(engine string) *app.Settings {
		s := &app.Settings{}
		s.DBUtils.Engine = engine

		return s
	}

	tests := []struct {
		name      string
		appName   string
		pipeline  bool
		reviewApp *string
		settings  *app.Settings
		want      bool
	}{
		{
			name:     "plain app with no engine gets the hint",
			appName:  "myapp",
			settings: settingsWithEngine(""),
			want:     true,
		},
		{
			// A managed AppPack database populates the engine, so the app already
			// has working db commands and must not be nagged.
			name:     "engine already set (managed database) stays quiet",
			appName:  "myapp",
			settings: settingsWithEngine("postgres"),
			want:     false,
		},
		{
			// Second `config set DATABASE_URL` after `modify app` already ran.
			name:     "engine already set (external database) stays quiet",
			appName:  "myapp",
			settings: settingsWithEngine("mysql"),
			want:     false,
		},
		{
			// External databases are gated on IsApp in CloudFormation, so telling a
			// pipeline to run `modify app` would be dead advice.
			name:     "pipeline stays quiet",
			appName:  "mypipeline",
			pipeline: true,
			settings: settingsWithEngine(""),
			want:     false,
		},
		{
			name:      "review app stays quiet",
			appName:   "mypipeline",
			reviewApp: aws.String("42"),
			settings:  settingsWithEngine(""),
			want:      false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			a := &app.App{
				Name:      tt.appName,
				Pipeline:  tt.pipeline,
				ReviewApp: tt.reviewApp,
				Settings:  tt.settings,
			}

			if got := shouldHintEnableDBUtils(a); got != tt.want {
				t.Errorf("shouldHintEnableDBUtils() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestDatabaseURLConfigVar pins the config variable name the hint keys off, since it
// has to match what the formations db-utils task definitions read from SSM.
func TestDatabaseURLConfigVar(t *testing.T) {
	t.Parallel()

	if databaseURLConfigVar != "DATABASE_URL" {
		t.Errorf("databaseURLConfigVar = %q, want %q", databaseURLConfigVar, "DATABASE_URL")
	}
}
