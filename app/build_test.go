package app_test

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/apppackio/apppack/app"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/logrusorgru/aurora"
)

func buildStatusFactory() map[string]*dynamodb.AttributeValue {
	return map[string]*dynamodb.AttributeValue{
		"app":          {S: aws.String("my-app")},
		"build_number": {N: aws.String("1")},
		"commit":       {S: aws.String("1234567890abcdef")},
		"status":       {S: aws.String("succeeded")},
		"build": {
			M: map[string]*dynamodb.AttributeValue{
				"arns":  {SS: []*string{aws.String("arn:aws:codebuild:us-east-1:123456789012:build/my-app:c0dc4b22-934d-4bd6-99e2-9eae50744bf7")}},
				"logs":  {S: aws.String("s3://my-app-build-artifacts/1/build.log")},
				"start": {N: aws.String("1679642449")},
				"end":   {N: aws.String("1679642990")},
				"state": {S: aws.String("succeeded")},
			},
		},
		"test": {
			M: map[string]*dynamodb.AttributeValue{
				"arns":  {SS: []*string{aws.String("arn:aws:codebuild:us-east-1:123456789012:build/my-app:c0dc4b22-934d-4bd6-99e2-9eae50744bf7")}},
				"logs":  {S: aws.String("s3://my-app-build-artifacts/1/test.log")},
				"start": {N: aws.String("1679642990")},
				"end":   {N: aws.String("1679643000")},
				"state": {S: aws.String("succeeded")},
			},
		},
		"finalize": {
			M: map[string]*dynamodb.AttributeValue{
				"arns":  {SS: []*string{aws.String("arn:aws:codebuild:us-east-1:123456789012:build/my-app:c0dc4b22-934d-4bd6-99e2-9eae50744bf7")}},
				"logs":  {S: aws.String("cloudwatch:///aws/codebuild/my-app#c0dc4b22-934d-4bd6-99e2-9eae50744bf7")},
				"start": {N: aws.String("1679643000")},
				"end":   {N: aws.String("1679643003")},
				"state": {S: aws.String("succeeded")},
			},
		},
		"release": {
			M: map[string]*dynamodb.AttributeValue{
				"arns":  {SS: []*string{aws.String("arn:aws:ecs:us-east-1:123456789012:task/apppack-cluster-apppack-EcsCluster-wbuTc2ColomY/7fd6a494ca204c399aab2be7fd3ca3ba")}},
				"logs":  {S: aws.String("s3://my-app-build-artifacts/1/release.log")},
				"start": {N: aws.String("1679643011")},
				"end":   {N: aws.String("1679643248")},
				"state": {S: aws.String("succeeded")},
			},
		},
		"deploy": {
			M: map[string]*dynamodb.AttributeValue{
				"arns":  {SS: []*string{aws.String("arn:aws:ecs:us-east-1:123456789012:service/apppack-cluster-apppack-EcsCluster-wbuTc2ColomY/my-app-web")}},
				"logs":  {S: aws.String("")},
				"start": {N: aws.String("1679643258")},
				"end":   {N: aws.String("1679643534")},
				"state": {S: aws.String("succeeded")},
			},
		},
	}
}

func TestBuildStatusStatus(t *testing.T) {
	t.Parallel()

	successState := buildStatusFactory()
	inProgressState := buildStatusFactory()
	inProgressState["deploy"] = &dynamodb.AttributeValue{M: map[string]*dynamodb.AttributeValue{}}
	failedState := buildStatusFactory()
	failedState["deploy"].M["state"].S = aws.String(app.PhaseFailed)

	scenarios := []struct {
		state    map[string]*dynamodb.AttributeValue
		expected string
	}{
		{state: successState, expected: app.PhaseSuccess},
		{state: inProgressState, expected: app.PhaseInProgress},
		{state: failedState, expected: app.PhaseFailed},
	}

	for _, scenario := range scenarios {
		s, err := app.NewBuildStatus(scenario.state)
		if err != nil {
			t.Error(err)
		}

		if s.Status != scenario.expected {
			t.Errorf("expected %s, got %s", scenario.expected, s.Status)
		}
	}
}

func TestBuildStatusFirstFailedState(t *testing.T) {
	t.Parallel()

	failedState := buildStatusFactory()
	failedState["test"].M["state"].S = aws.String(app.PhaseFailed)

	s, err := app.NewBuildStatus(failedState)
	if err != nil {
		t.Error(err)
	}

	expected := app.TestPhaseName
	actual := s.FirstFailedPhase().Name

	if actual != expected {
		t.Errorf("expected %s, got %s", expected, actual)
	}
}

func TestBuildStatusFirstFailedStateEmpty(t *testing.T) {
	t.Parallel()

	s, err := app.NewBuildStatus(buildStatusFactory())
	if err != nil {
		t.Error(err)
	}

	actual := s.FirstFailedPhase()
	if actual != nil {
		t.Errorf("expected nil, got %s", actual.Name)
	}
}

func TestBuildStatusFinalPhase(t *testing.T) {
	t.Parallel()

	deploy, err := app.NewBuildStatus(buildStatusFactory())
	if err != nil {
		t.Error(err)
	}

	test, err := app.NewBuildStatus(buildStatusFactory())
	if err != nil {
		t.Error(err)
	}

	test.Deploy = nil
	test.Release = nil
	test.Finalize = nil

	scenarios := []struct {
		s        *app.BuildStatus
		expected app.BuildPhaseLabel
	}{
		{s: deploy, expected: app.DeployPhaseName},
		{s: test, expected: app.TestPhaseName},
	}
	for _, scenario := range scenarios {
		actual, err := scenario.s.FinalPhase()
		if err != nil {
			t.Error(err)
		}

		if actual.Name != scenario.expected {
			t.Errorf("expected %s, got %s", scenario.expected, actual.Name)
		}
	}

	deploy.Deploy.State = app.PhaseInProgress
	deploy.Deploy.End = 0

	_, err = deploy.FinalPhase()
	if err == nil {
		t.Error("expected error for in progress phase")
	}
}

func TestBuildStatusNexActivePhase(t *testing.T) {
	t.Parallel()

	s, err := app.NewBuildStatus(buildStatusFactory())
	if err != nil {
		t.Error(err)
	}

	scenarios := []struct {
		in       *app.BuildPhase
		expected app.BuildPhaseLabel
	}{
		{in: &app.BuildPhase{Name: app.BuildPhaseName, Phase: s.Build}, expected: app.TestPhaseName},
		{in: &app.BuildPhase{Name: app.TestPhaseName, Phase: s.Test}, expected: app.FinalizePhaseName},
	}

	for _, scenario := range scenarios {
		actual := s.NextActivePhase(scenario.in)
		if actual == nil {
			t.Error("expected phase, got nil")

			continue
		}

		if actual.Name != scenario.expected {
			t.Errorf("expected %s, got %s", scenario.expected, actual.Name)
		}
	}
}

func TestBuildStatusCurrentPhase(t *testing.T) {
	t.Parallel()

	s, err := app.NewBuildStatus(buildStatusFactory())
	if err != nil {
		t.Error(err)
	}

	s.Test.State = app.PhaseInProgress
	expected := app.TestPhaseName

	actual := s.CurrentPhase()
	if actual == nil {
		t.Error("expected phase, got nil")

		return
	}

	if actual.Name != expected {
		t.Errorf("expected %s, got %s", expected, actual.Name)
	}
}

func TestBuildStatuses(t *testing.T) {
	t.Parallel()

	s, err := app.NewBuildStatuses([]map[string]*dynamodb.AttributeValue{buildStatusFactory(), buildStatusFactory()})
	if err != nil {
		t.Error(err)
	}

	if len(s) != 2 {
		t.Errorf("expected 2, got %d", len(s))
	}
}

func TestTimes(t *testing.T) {
	t.Parallel()

	s, err := app.NewBuildStatus(buildStatusFactory())
	if err != nil {
		t.Error(err)
	}

	scenarios := []struct {
		f        func() time.Time
		expected time.Time
	}{
		{f: s.Build.StartTime, expected: time.Date(2023, time.March, 24, 7, 20, 49, 0, time.UTC)},
		{f: s.Build.EndTime, expected: time.Date(2023, time.March, 24, 7, 29, 50, 0, time.UTC)},
	}

	for _, scenario := range scenarios {
		actual := scenario.f().UTC()
		if actual != scenario.expected {
			t.Errorf("expected %s, got %s", scenario.expected, actual)
		}
	}
}

func TestBuildStatusToConsole(t *testing.T) {
	t.Parallel()

	s, err := app.NewBuildStatus(buildStatusFactory())
	if err != nil {
		t.Error(err)
	}

	var buf bytes.Buffer

	s.ToConsole(&buf)
	actual := buf.String()
	check := aurora.Green("✔").String()
	expected := strings.Join([]string{
		check + " Build",
		check + " Test",
		check + " Finalize",
		check + " Release",
		check + " Deploy",
	},
		aurora.Faint("  |  ").String(),
	)
	// verify that `expected` is a substring of `actual`
	if !strings.Contains(actual, expected) {
		t.Errorf("expected %s, got %s", expected, actual)
	}
}
