package diagnose

import (
	"fmt"
	"strings"

	"github.com/apppackio/apppack/app"
)

// PhaseState is one build phase and its outcome.
type PhaseState struct {
	Name  string
	State string
}

// TaskDefSummary describes a service's task definition. Environment variable
// NAMES are included; values never are.
type TaskDefSummary struct {
	Service     string
	Image       string
	Command     []string
	CPU         string
	Memory      string
	Env         []string
	HealthCheck string
}

// Context is the evidence preloaded into the model's first message: small,
// structured, and always relevant. Large evidence (logs) is pulled through
// tools instead.
type Context struct {
	AppName     string
	Region      string
	Pipeline    bool
	BuildNumber *int
	Phases      []PhaseState
	Services    []string
	ConfigKeys  []string
	TaskDefs    []TaskDefSummary
}

// phaseNames is the ordered list of build phases, matching
// app.BuildStatus.NamedPhases.
var phaseNames = []string{"build", "test", "finalize", "release", "postdeploy", "deploy"}

// PhaseStates flattens a BuildStatus into ordered phase outcomes.
func PhaseStates(b *app.BuildStatus) []PhaseState {
	if b == nil {
		return nil
	}

	details := []*app.BuildPhaseDetail{
		&b.Build, &b.Test, &b.Finalize, &b.Release, &b.Postdeploy, &b.Deploy,
	}
	names := []string{"Build", "Test", "Finalize", "Release", "Postdeploy", "Deploy"}

	states := make([]PhaseState, 0, len(details))
	for i, d := range details {
		states = append(states, PhaseState{Name: names[i], State: d.State})
	}

	return states
}

// PhaseLogURL returns the S3 URL holding a phase's log.
//
// Phases that never ran have an empty Logs field. Returning that to
// app.S3FromURL would parse it into a garbage bucket name, so it is rejected
// here with a message the model can act on.
func PhaseLogURL(b *app.BuildStatus, phase string) (string, error) {
	if b == nil {
		return "", fmt.Errorf("no build is available, so there is no %s log", phase)
	}

	var detail *app.BuildPhaseDetail

	switch strings.ToLower(phase) {
	case "build":
		detail = &b.Build
	case "test":
		detail = &b.Test
	case "finalize":
		detail = &b.Finalize
	case "release":
		detail = &b.Release
	case "postdeploy":
		detail = &b.Postdeploy
	case "deploy":
		detail = &b.Deploy
	default:
		return "", fmt.Errorf("unknown phase %q: must be one of %s", phase, strings.Join(phaseNames, ", "))
	}

	if !strings.HasPrefix(detail.Logs, "s3://") {
		return "", fmt.Errorf("no log is available for the %s phase (it may not have run)", phase)
	}

	return detail.Logs, nil
}

// Render formats the preloaded context for the model's first message.
func (c *Context) Render() string {
	var b strings.Builder

	fmt.Fprintf(&b, "App: %s\nRegion: %s\n", c.AppName, c.Region)

	if c.Pipeline {
		b.WriteString("This app is a pipeline.\n")
	}

	if c.BuildNumber != nil {
		fmt.Fprintf(&b, "Build number: %d\n", *c.BuildNumber)
	}

	b.WriteString("\n## Build phases\n")

	if len(c.Phases) == 0 {
		b.WriteString("No build was found for this app. Diagnose its current running state instead.\n")
	} else {
		for _, p := range c.Phases {
			state := p.State
			if state == "" {
				state = "did not run"
			}

			fmt.Fprintf(&b, "- %s: %s\n", p.Name, state)
		}
	}

	b.WriteString("\n## Services\n")

	if len(c.Services) == 0 {
		b.WriteString("No services are running. The app has never completed a successful release.\n")
	} else {
		for _, s := range c.Services {
			fmt.Fprintf(&b, "- %s\n", s)
		}
	}

	b.WriteString("\n## Config variables (names only, values are never read)\n")

	if len(c.ConfigKeys) == 0 {
		b.WriteString("None are set.\n")
	} else {
		for _, k := range c.ConfigKeys {
			fmt.Fprintf(&b, "- %s\n", k)
		}
	}

	if len(c.TaskDefs) > 0 {
		b.WriteString("\n## Task definitions\n")

		for _, td := range c.TaskDefs {
			fmt.Fprintf(&b, "\n### %s\n", td.Service)
			fmt.Fprintf(&b, "- image: %s\n", td.Image)
			fmt.Fprintf(&b, "- command: %s\n", strings.Join(td.Command, " "))
			fmt.Fprintf(&b, "- cpu/memory: %s/%s\n", td.CPU, td.Memory)
			fmt.Fprintf(&b, "- health check: %s\n", td.HealthCheck)
			fmt.Fprintf(&b, "- env: %s\n", strings.Join(td.Env, ", "))
		}
	}

	return b.String()
}
