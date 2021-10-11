package app

import (
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws/session"
)

type BuildPhaseDetail struct {
	Arns  []string `json:"arns"`
	Logs  string   `json:"logs"`
	Start int64    `json:"start"`
	End   int64    `json:"end"`
	State string   `json:"state"`
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
	AppName     string           `json:"app"`
	BuildNumber int              `json:"build_number"`
	PRNumber    string           `json:"pr_number"`
	Commit      string           `json:"commit"`
	Build       BuildPhaseDetail `json:"build"`
	Test        BuildPhaseDetail `json:"test"`
	Finalize    BuildPhaseDetail `json:"finalize"`
	Release     BuildPhaseDetail `json:"release"`
	Postdeploy  BuildPhaseDetail `json:"postdeploy"`
	Deploy      BuildPhaseDetail `json:"deploy"`
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
	return nil, fmt.Errorf("no phases completed")
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
func (b *BuildStatus) GetCommitLog(sess *session.Session) (*string, error) {
	if b.Build.Logs == "" || !strings.HasPrefix(b.Build.Logs, "s3://") {
		return nil, fmt.Errorf("build logs not available yet")
	}
	s3Parts := strings.Split(b.Build.Logs, "/")
	s3Parts = append(s3Parts[0:len(s3Parts)-1], "commit.txt")
	commitURL := strings.Join(s3Parts, "/")
	builder, err := S3FromURL(sess, commitURL)
	if err != nil {
		return nil, err
	}
	contents := builder.String()
	return &contents, nil
}
