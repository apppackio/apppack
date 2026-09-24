package diagnose_test

import (
	"testing"

	"github.com/apppackio/apppack/app"
	"github.com/apppackio/apppack/diagnose"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPhaseStates(t *testing.T) {
	t.Parallel()

	b := &app.BuildStatus{
		Build:  app.BuildPhaseDetail{State: app.PhaseSuccess},
		Test:   app.BuildPhaseDetail{State: app.PhaseSuccess},
		Deploy: app.BuildPhaseDetail{State: app.PhaseFailed},
	}

	states := diagnose.PhaseStates(b)
	require.Len(t, states, 6)
	assert.Equal(t, diagnose.PhaseState{Name: "Build", State: app.PhaseSuccess}, states[0])
	assert.Equal(t, diagnose.PhaseState{Name: "Deploy", State: app.PhaseFailed}, states[5])
}

func TestPhaseLogURL(t *testing.T) {
	t.Parallel()

	b := &app.BuildStatus{
		Build: app.BuildPhaseDetail{Logs: "s3://bucket/build.log"},
	}

	got, err := diagnose.PhaseLogURL(b, "build")
	require.NoError(t, err)
	assert.Equal(t, "s3://bucket/build.log", got)
}

// Review Focus 2: phases that never ran have an empty Logs field. Passing that
// to S3FromURL would parse "" into a garbage bucket name.
func TestPhaseLogURLMissingLog(t *testing.T) {
	t.Parallel()

	b := &app.BuildStatus{
		Build:   app.BuildPhaseDetail{Logs: ""},
		Release: app.BuildPhaseDetail{Logs: "https://example.com/not-s3"},
	}

	_, err := diagnose.PhaseLogURL(b, "build")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no log")

	_, err = diagnose.PhaseLogURL(b, "release")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no log")

	_, err = diagnose.PhaseLogURL(b, "nonsense")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown phase")
}

// Review Focus 1: a brand-new app whose first deploy failed has no successful
// release, so there are no services and no build phases.
func TestContextRenderWithNoBuild(t *testing.T) {
	t.Parallel()

	c := diagnose.Context{
		AppName:    "myapp",
		Region:     "us-east-1",
		Phases:     nil,
		Services:   nil,
		ConfigKeys: []string{"DATABASE_URL"},
	}

	out := c.Render()
	assert.Contains(t, out, "myapp")
	assert.Contains(t, out, "DATABASE_URL")
	assert.Contains(t, out, "No build was found")
	assert.Contains(t, out, "No services are running")
	assert.NotContains(t, out, "succeeded")
}

func TestContextRenderOmitsConfigValues(t *testing.T) {
	t.Parallel()

	c := diagnose.Context{
		AppName:    "myapp",
		Region:     "us-east-1",
		ConfigKeys: []string{"SECRET_KEY", "DATABASE_URL"},
	}

	out := c.Render()
	assert.Contains(t, out, "SECRET_KEY")
	assert.Contains(t, out, "names only")
}
