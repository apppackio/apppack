package app

import (
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
