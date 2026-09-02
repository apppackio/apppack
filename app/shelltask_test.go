package app

import (
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	ecstypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"
)

// seedShellTask pre-populates the cached shell task definition and marks the
// sync.Once consumed, so IsBuildpack/ShellTaskFamily read this data instead of
// making an AWS call.
func seedShellTask(family string, tags []ecstypes.Tag) *App {
	a := &App{}
	a.shellOnce.Do(func() {
		a.shellTask.taskFamily = family
		a.shellTask.tags = tags
	})

	return a
}

func buildSystemTag(value string) ecstypes.Tag {
	return ecstypes.Tag{Key: aws.String("apppack:buildSystem"), Value: aws.String(value)}
}

func TestIsBuildpack(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		tags []ecstypes.Tag
		want bool
	}{
		{
			name: "buildpacks tag",
			tags: []ecstypes.Tag{buildSystemTag("buildpacks")},
			want: true,
		},
		{
			name: "empty tag value treated as buildpacks",
			tags: []ecstypes.Tag{buildSystemTag("")},
			want: true,
		},
		{
			name: "missing tag treated as buildpacks",
			tags: []ecstypes.Tag{{Key: aws.String("apppack:appName"), Value: aws.String("myapp")}},
			want: true,
		},
		{
			name: "no tags at all treated as buildpacks",
			tags: nil,
			want: true,
		},
		{
			name: "docker build system",
			tags: []ecstypes.Tag{buildSystemTag("docker")},
			want: false,
		},
		{
			name: "build system tag among others",
			tags: []ecstypes.Tag{
				{Key: aws.String("apppack:appName"), Value: aws.String("myapp")},
				buildSystemTag("docker"),
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			a := seedShellTask("myapp-shell", tt.tags)

			got, err := a.IsBuildpack()
			if err != nil {
				t.Fatalf("IsBuildpack() unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("IsBuildpack() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestShellTaskFamily(t *testing.T) {
	t.Parallel()

	a := seedShellTask("myapp-shell", nil)

	family, err := a.ShellTaskFamily()
	if err != nil {
		t.Fatalf("ShellTaskFamily() unexpected error: %v", err)
	}
	if family == nil {
		t.Fatal("ShellTaskFamily() = nil, want pointer to \"myapp-shell\"")
	}
	if *family != "myapp-shell" {
		t.Errorf("ShellTaskFamily() = %q, want %q", *family, "myapp-shell")
	}
}

// TestDBShellTaskInfo covers the exec command DBShellTaskInfo builds per engine.
// Settings is pre-populated in every case, which makes App.LoadSettings
// short-circuit, so none of these touch AWS -- same technique as the tests in
// cmd/config_test.go.
func TestDBShellTaskInfo(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		engine     string
		reviewApp  *string
		wantExec   string
		wantErrSub string // non-empty: error must contain this substring
	}{
		{
			// A managed AppPack database's Engine field is indistinguishable
			// from an external one -- both are just "mysql" -- so a bare
			// `mysql` must keep working for it too.
			name:     "managed mysql gets a bare mysql",
			engine:   "mysql",
			wantExec: "mysql",
		},
		{
			// The db-utils image resolves the database from DATABASE_URL via
			// ~/.my.cnf, so no --database flag is needed (or correct) here.
			name:     "external mysql gets a bare mysql",
			engine:   "mysql",
			wantExec: "mysql",
		},
		{
			name:     "postgres is untouched by this change",
			engine:   "postgres",
			wantExec: "psql",
		},
		{
			// Review apps cannot use external databases (the CloudFormation
			// condition requires IsApp), so this path must stay byte-identical
			// to its pre-existing behavior.
			name:      "review app mysql keeps the explicit --database form",
			engine:    "mysql",
			reviewApp: aws.String("42"),
			wantExec:  "mysql --database=myapp-pr42 myapp-pr42",
		},
		{
			name:       "empty engine surfaces the no-database-configured error",
			engine:     "",
			wantErrSub: "no database is configured",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			a := &App{Name: "myapp", ReviewApp: tt.reviewApp, Settings: &Settings{}}
			a.Settings.DBUtils.Engine = tt.engine
			a.Settings.DBUtils.ShellTaskFamily = "myapp-dbshell"

			_, exec, err := a.DBShellTaskInfo()

			if tt.wantErrSub != "" {
				if err == nil {
					t.Fatal("expected an error, got nil")
				}
				if !strings.Contains(err.Error(), tt.wantErrSub) {
					t.Errorf("error %q does not contain %q", err.Error(), tt.wantErrSub)
				}

				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if exec == nil {
				t.Fatal("exec = nil, want a command")
			}
			if *exec != tt.wantExec {
				t.Errorf("exec = %q, want %q", *exec, tt.wantExec)
			}
		})
	}
}
