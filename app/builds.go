package app

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
)

type BuildPhaseDetail struct {
	Arns  []string `dynamodbav:"arns"  json:"arns"`
	Logs  string   `dynamodbav:"logs"  json:"logs"`
	Start int64    `dynamodbav:"start" json:"start"`
	End   int64    `dynamodbav:"end"   json:"end"`
	State string   `dynamodbav:"state" json:"state"`
}

func (b *BuildPhaseDetail) StartTime() time.Time {
	return time.Unix(b.Start, 0)
}

func (b *BuildPhaseDetail) EndTime() time.Time {
	return time.Unix(b.End, 0)
}

const (
	PhaseSuccess    = "succeeded"
	PhaseFailed     = "failed"
	PhaseInProgress = "started"
)

type BuildStatus struct {
	AppName     string           `dynamodbav:"app"          json:"app"`
	BuildNumber int              `dynamodbav:"build_number" json:"build_number"`
	PRNumber    string           `dynamodbav:"pr_number"    json:"pr_number"`
	Commit      string           `dynamodbav:"commit"       json:"commit"`
	Build       BuildPhaseDetail `dynamodbav:"build"        json:"build"`
	Test        BuildPhaseDetail `dynamodbav:"test"         json:"test"`
	Finalize    BuildPhaseDetail `dynamodbav:"finalize"     json:"finalize"`
	Release     BuildPhaseDetail `dynamodbav:"release"      json:"release"`
	Postdeploy  BuildPhaseDetail `dynamodbav:"postdeploy"   json:"postdeploy"`
	Deploy      BuildPhaseDetail `dynamodbav:"deploy"       json:"deploy"`
}

type BuildPhase struct {
	Name  string
	Phase *BuildPhaseDetail
}

func (b *BuildStatus) NamedPhases() [6]BuildPhase {
	return [6]BuildPhase{
		{Name: "Build", Phase: &b.Build},
		{Name: "Test", Phase: &b.Test},
		{Name: "Finalize", Phase: &b.Finalize},
		{Name: "Release", Phase: &b.Release},
		{Name: "Postdeploy", Phase: &b.Postdeploy},
		{Name: "Deploy", Phase: &b.Deploy},
	}
}

func (b *BuildStatus) NamedPhasesReversed() [6]BuildPhase {
	return [6]BuildPhase{
		{Name: "Deploy", Phase: &b.Deploy},
		{Name: "Postdeploy", Phase: &b.Postdeploy},
		{Name: "Release", Phase: &b.Release},
		{Name: "Finalize", Phase: &b.Finalize},
		{Name: "Test", Phase: &b.Test},
		{Name: "Build", Phase: &b.Build},
	}
}

func (b *BuildStatus) CurrentPhase() *BuildPhase {
	for _, p := range b.NamedPhases() {
		if p.Phase.State == PhaseInProgress {
			return &p
		}
	}

	return nil
}

// NextActivePhase finds the next phase which already ran or is in progress
func (b *BuildStatus) NextActivePhase(lastPhase *BuildPhase) *BuildPhase {
	found := false
	for _, p := range b.NamedPhases() {
		if found && p.Phase.Start != 0 {
			return &p
		}

		if !found && p.Name == lastPhase.Name {
			found = true
		}
	}

	if b.Deploy.End != 0 {
		return nil
	}

	return lastPhase
}

func (b *BuildStatus) FinalPhase() (*BuildPhase, error) {
	for _, p := range b.NamedPhasesReversed() {
		if p.Phase.State == PhaseInProgress {
			return nil, fmt.Errorf("%s phase is still running", p.Name)
		}

		if p.Phase.State == PhaseSuccess || p.Phase.State == PhaseFailed {
			return &p, nil
		}
	}

	return nil, errors.New("no phases completed")
}

func (b *BuildStatus) FirstFailedPhase() *BuildPhase {
	for _, p := range b.NamedPhases() {
		if p.Phase.State == PhaseFailed {
			return &p
		}
	}

	return nil
}

// GetCommitLog retrieves commit.txt stored in S3
func (b *BuildStatus) GetCommitLog(cfg aws.Config) (*string, error) {
	if b.Build.Logs == "" || !strings.HasPrefix(b.Build.Logs, "s3://") {
		return nil, errors.New("build logs not available yet")
	}

	s3Parts := strings.Split(b.Build.Logs, "/")
	s3Parts = append(s3Parts[0:len(s3Parts)-1], "commit.txt")
	commitURL := strings.Join(s3Parts, "/")

	builder, err := S3FromURL(cfg, commitURL)
	if err != nil {
		return nil, err
	}

	contents := builder.String()

	return &contents, nil
}
