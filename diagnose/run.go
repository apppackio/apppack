package diagnose

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/apppackio/apppack/app"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/bedrock"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	ecstypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"
)

// Diagnose gathers evidence for an app and returns the model's diagnosis.
//
// The answer is returned unmodified. There is deliberately no client-side
// redaction: see "Task 2 — REMOVED" in the plan and the spec's decision
// section. The no-credential-echo rule in the system prompt is the control.
func Diagnose(ctx context.Context, a *app.App, buildNumber *int, modelID string) (string, error) {
	region := a.Session.Region

	geo, err := GeographyForRegion(region)
	if err != nil {
		return "", err
	}

	// Resolve the model ID to invoke.
	//
	// With no --model, discover the inference profile for the pinned model
	// that keeps inference inside the app's geography (Task 11). The prefix
	// is per-model and AWS revises it -- Claude Sonnet 5 has us./eu./au./
	// global. and no apac. at all -- so it must not be hardcoded.
	//
	// With --model, the user's value is used as given (prefixed if bare),
	// bypassing discovery entirely.
	resolvedModel := modelID
	if resolvedModel == "" {
		resolvedModel, err = SelectProfile(
			ctx, bedrock.NewFromConfig(a.Session), geo, DefaultModelID,
		)
		if err != nil {
			return "", TranslateError(err, a.Name, a.Pipeline, region)
		}
	} else {
		resolvedModel = ModelIDForGeography(geo, resolvedModel)
	}

	buildStatus := loadBuildStatus(a, buildNumber)

	services, err := a.GetServices()
	if err != nil {
		return "", err
	}

	configKeys, err := a.GetConfigKeys()
	if err != nil {
		return "", err
	}

	dctx := Context{
		AppName:    a.Name,
		Region:     region,
		Pipeline:   a.Pipeline,
		Services:   services,
		ConfigKeys: configKeys,
		Phases:     PhaseStates(buildStatus),
		TaskDefs:   taskDefSummaries(a, services),
	}

	if buildStatus != nil {
		n := buildStatus.BuildNumber
		dctx.BuildNumber = &n
	}

	registry := NewRegistry(BuildTools(ToolDeps{
		Services:       services,
		PhaseLog:       func(phase string) (string, error) { return phaseLog(a, buildStatus, phase) },
		AppLogs:        func(service string, since, limit int) (string, error) { return appLogs(ctx, a, service, since, limit) },
		ECSEvents:      func(service string) (string, error) { return ecsEvents(a, service) },
		DescribeTasks:  func(service string) (string, error) { return describeTasks(a, service) },
		TaskDefinition: func(service string) (string, error) { return taskDefinition(a, service) },
	}))

	client := bedrockruntime.NewFromConfig(a.Session)

	answer, err := Run(ctx, client, resolvedModel, SystemPrompt(), dctx.Render(), registry)
	if err != nil {
		return "", TranslateError(err, a.Name, a.Pipeline, region)
	}

	return answer, nil
}

// loadBuildStatus returns the requested build, the most recent build, or nil
// when the app has never been built. A missing build is not an error: the
// command falls back to diagnosing current state.
func loadBuildStatus(a *app.App, buildNumber *int) *app.BuildStatus {
	if buildNumber != nil {
		b, err := a.GetBuildStatus(*buildNumber)
		if err != nil {
			return nil
		}

		return b
	}

	builds, err := a.RecentBuilds(1)
	if err != nil || len(builds) == 0 {
		return nil
	}

	return &builds[0]
}

func phaseLog(a *app.App, b *app.BuildStatus, phase string) (string, error) {
	url, err := PhaseLogURL(b, phase)
	if err != nil {
		return "", err
	}

	contents, err := app.S3FromURL(a.Session, url)
	if err != nil {
		return "", fmt.Errorf("could not read the %s log: %w", phase, err)
	}

	return contents.String(), nil
}

func appLogs(ctx context.Context, a *app.App, service string, sinceMinutes, limit int) (string, error) {
	if err := a.LoadSettings(); err != nil {
		return "", err
	}

	svc := cloudwatchlogs.NewFromConfig(a.Session)
	start := time.Now().Add(-time.Duration(sinceMinutes) * time.Minute).UnixMilli()

	out, err := svc.FilterLogEvents(ctx, &cloudwatchlogs.FilterLogEventsInput{
		LogGroupName:        aws.String(a.Settings.LogGroup.Name),
		LogStreamNamePrefix: aws.String(service),
		StartTime:           aws.Int64(start),
		// #nosec G115 -- limit reaches here only via ValidateInt, which
		// clamps it to [1, 1000] before any tool can pass it on.
		Limit: aws.Int32(int32(limit)),
	})
	if err != nil {
		return "", err
	}

	if len(out.Events) == 0 {
		return fmt.Sprintf("No log events for %s in the last %d minutes.", service, sinceMinutes), nil
	}

	var b strings.Builder

	for _, e := range out.Events {
		fmt.Fprintf(&b, "%s %s\n",
			time.UnixMilli(aws.ToInt64(e.Timestamp)).UTC().Format(time.RFC3339),
			strings.TrimRight(aws.ToString(e.Message), "\n"),
		)
	}

	return b.String(), nil
}

func ecsEvents(a *app.App, service string) (string, error) {
	events, err := a.GetECSEvents(service)
	if err != nil {
		return "", err
	}

	if len(events) == 0 {
		return fmt.Sprintf("No ECS service events for %s.", service), nil
	}

	var b strings.Builder

	for _, e := range events {
		fmt.Fprintf(&b, "%s %s\n", e.CreatedAt.UTC().Format(time.RFC3339), aws.ToString(e.Message))
	}

	return b.String(), nil
}

func describeTasks(a *app.App, service string) (string, error) {
	tasks, err := a.DescribeTasks()
	if err != nil {
		return "", err
	}

	type taskSummary struct {
		TaskARN       string `json:"task_arn"`
		LastStatus    string `json:"last_status"`
		DesiredStatus string `json:"desired_status"`
		HealthStatus  string `json:"health_status"`
		StoppedReason string `json:"stopped_reason,omitempty"`
		Containers    []struct {
			Name     string `json:"name"`
			ExitCode *int32 `json:"exit_code,omitempty"`
			Reason   string `json:"reason,omitempty"`
		} `json:"containers"`
	}

	var summaries []taskSummary

	// ECS sets a service task's Group to "service:<ecs-service-name>", where
	// the ECS service name is the qualified form from a.ServiceName. Compare
	// exactly: a substring match would mis-attribute tasks whenever one
	// process name is a substring of another.
	wantGroup := "service:" + a.ServiceName(service)

	for i := range tasks {
		t := tasks[i]
		if aws.ToString(t.Group) != wantGroup {
			continue
		}

		s := taskSummary{
			TaskARN:       aws.ToString(t.TaskArn),
			LastStatus:    aws.ToString(t.LastStatus),
			DesiredStatus: aws.ToString(t.DesiredStatus),
			HealthStatus:  string(t.HealthStatus),
			StoppedReason: aws.ToString(t.StoppedReason),
		}

		for j := range t.Containers {
			c := t.Containers[j]
			s.Containers = append(s.Containers, struct {
				Name     string `json:"name"`
				ExitCode *int32 `json:"exit_code,omitempty"`
				Reason   string `json:"reason,omitempty"`
			}{aws.ToString(c.Name), c.ExitCode, aws.ToString(c.Reason)})
		}

		summaries = append(summaries, s)
	}

	if len(summaries) == 0 {
		return fmt.Sprintf("No tasks found for %s. The service may have no running or recently stopped tasks.", service), nil
	}

	out, err := json.MarshalIndent(summaries, "", "  ")
	if err != nil {
		return "", err
	}

	return string(out), nil
}

// taskDefinition reads a service's task definition.
//
// Pass the BARE process name ("web"), not a.ServiceName(service):
// App.TaskDefinition applies ServiceName internally (app/app.go:340), so
// qualifying it here would produce "myapp-myapp-web" and fail to resolve.
func taskDefinition(a *app.App, service string) (string, error) {
	td, _, err := a.TaskDefinition(service)
	if err != nil {
		return "", err
	}

	out, err := json.MarshalIndent(summarizeTaskDefinition(td), "", "  ")
	if err != nil {
		return "", err
	}

	return string(out), nil
}

// summarizeTaskDefinition reduces a task definition to the fields that help
// diagnose a failure, dropping ARNs, network config, and other noise.
//
// Environment variable values are included. AppPack never writes secrets into
// task definitions -- config is delivered through SSM, which this package
// reads names-only (see App.GetConfigKeys) -- so these values are not
// sensitive, and a wrong one (a bad PORT, a stale hostname) is a common cause
// of the failures this tool exists to diagnose. `secrets` entries carry an
// SSM/Secrets Manager reference rather than a value, so they are safe too.
func summarizeTaskDefinition(td *ecstypes.TaskDefinition) map[string]any {
	containers := make([]map[string]any, 0, len(td.ContainerDefinitions))

	for i := range td.ContainerDefinitions {
		c := td.ContainerDefinitions[i]

		env := make(map[string]string, len(c.Environment))
		for _, e := range c.Environment {
			env[aws.ToString(e.Name)] = aws.ToString(e.Value)
		}

		secretRefs := make(map[string]string, len(c.Secrets))
		for _, s := range c.Secrets {
			secretRefs[aws.ToString(s.Name)] = aws.ToString(s.ValueFrom)
		}

		containers = append(containers, map[string]any{
			"name":               aws.ToString(c.Name),
			"image":              aws.ToString(c.Image),
			"command":            c.Command,
			"entry_point":        c.EntryPoint,
			"cpu":                c.Cpu,
			"memory":             c.Memory,
			"memory_reservation": c.MemoryReservation,
			"port_mappings":      c.PortMappings,
			"health_check":       c.HealthCheck,
			"environment":        env,
			"secret_references":  secretRefs,
		})
	}

	return map[string]any{
		"family":                aws.ToString(td.Family),
		"revision":              td.Revision,
		"cpu":                   aws.ToString(td.Cpu),
		"memory":                aws.ToString(td.Memory),
		"container_definitions": containers,
	}
}

func taskDefSummaries(a *app.App, services []string) []TaskDefSummary {
	var out []TaskDefSummary

	for _, s := range services {
		// Bare process name — TaskDefinition qualifies it internally.
		td, _, err := a.TaskDefinition(s)
		if err != nil || len(td.ContainerDefinitions) == 0 {
			continue
		}

		c := td.ContainerDefinitions[0]

		// Values included: AppPack never writes secrets into task
		// definitions, and a wrong value here is a common failure cause.
		env := make([]string, 0, len(c.Environment))
		for _, e := range c.Environment {
			env = append(env, fmt.Sprintf("%s=%s", aws.ToString(e.Name), aws.ToString(e.Value)))
		}

		health := "none"
		if c.HealthCheck != nil {
			health = strings.Join(c.HealthCheck.Command, " ")
		}

		out = append(out, TaskDefSummary{
			Service:     s,
			Image:       aws.ToString(c.Image),
			Command:     c.Command,
			CPU:         aws.ToString(td.Cpu),
			Memory:      aws.ToString(td.Memory),
			Env:         env,
			HealthCheck: health,
		})
	}

	return out
}
