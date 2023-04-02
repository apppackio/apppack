package app

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/apppackio/apppack/ui"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/dynamodb/dynamodbattribute"
	"github.com/dustin/go-humanize"
	"github.com/logrusorgru/aurora"
)

type BuildPhaseLabel string

var (
	ErrUnknownPhase                     = fmt.Errorf("unknown phase name")
	BuildPhaseName      BuildPhaseLabel = "Build"
	TestPhaseName       BuildPhaseLabel = "Test"
	FinalizePhaseName   BuildPhaseLabel = "Finalize"
	ReleasePhaseName    BuildPhaseLabel = "Release"
	PostdeployPhaseName BuildPhaseLabel = "Postdeploy"
	DeployPhaseName     BuildPhaseLabel = "Deploy"
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

var (
	ErrBuildListEmpty        = fmt.Errorf("build list is empty")
	ErrBuildInProgress       = fmt.Errorf("build still in progress")
	ErrBuildLogsNotAvailable = fmt.Errorf("build logs not available")
)

func statusIcon(status string) string {
	switch status {
	case PhaseSuccess:
		return aurora.Green("✔").String()
	case PhaseFailed:
		return aurora.Red("✖").String()
	case PhaseInProgress:
		return aurora.Faint(aurora.Blue("ℹ")).String()
	default:
		return ""
	}
}

type BuildStatus struct {
	AppName     string            `json:"app"`
	BuildNumber int               `json:"build_number"`
	PRNumber    string            `json:"pr_number,omitempty"`
	Commit      string            `json:"commit"`
	Status      string            `json:"status"` // calculated on import
	Build       *BuildPhaseDetail `json:"build"`
	Test        *BuildPhaseDetail `json:"test"`
	Finalize    *BuildPhaseDetail `json:"finalize"`
	Release     *BuildPhaseDetail `json:"release"`
	Postdeploy  *BuildPhaseDetail `json:"postdeploy,omitempty"`
	Deploy      *BuildPhaseDetail `json:"deploy"`
}

func (b *BuildStatus) ToConsole(w io.Writer) {
	ui.PrintHeader(fmt.Sprintf("%d", b.BuildNumber))
	currentPhase := b.CurrentPhase()

	if currentPhase != nil {
		fmt.Fprint(w, " in progress")
	}

	fmt.Fprint(w, " ", aurora.Blue(b.Commit))
	fmt.Fprintf(w, "\n%s", ui.Indent)
	finalPhase, _ := b.FinalPhase()

	for _, p := range b.NamedPhases() {
		if p.Phase == nil || p.Phase.State == "" {
			continue
		}

		fmt.Fprintf(w, "%s %s", statusIcon(p.Phase.State), p.Name)

		if (currentPhase != nil && p.Name == currentPhase.Name) || (finalPhase != nil && p.Name == finalPhase.Name) {
			fmt.Fprintln(w)

			break
		}

		fmt.Fprint(w, aurora.Faint("  |  ").String())
	}

	if finalPhase != nil {
		fmt.Fprintln(w, aurora.Faint(fmt.Sprintf(
			"%s%s ~ %s",
			ui.Indent,
			finalPhase.Phase.EndTime().Local().Format(ui.TimeFmt),
			humanize.Time(finalPhase.Phase.EndTime()),
		)))
	} else {
		fmt.Fprintln(w, aurora.Faint(fmt.Sprintf(
			"%sstarted %s ~ %s",
			ui.Indent,
			b.Build.StartTime().Local().Format(ui.TimeFmt),
			humanize.Time(b.Build.StartTime()),
		)))
	}
}

func (b *BuildStatus) ToJSON() (*bytes.Buffer, error) {
	return toJSON(b)
}

func (b *BuildStatus) SetStatus() {
	if b.Deploy != nil && b.Deploy.State == PhaseSuccess {
		b.Status = PhaseSuccess

		return
	}

	for _, p := range b.NamedPhases() {
		if p.Phase != nil && p.Phase.State == PhaseFailed {
			b.Status = PhaseFailed

			return
		}
	}

	b.Status = PhaseInProgress
}

func NewBuildStatus(item map[string]*dynamodb.AttributeValue) (*BuildStatus, error) {
	if item == nil {
		return nil, ErrBuildListEmpty
	}
	var build BuildStatus

	if err := dynamodbattribute.UnmarshalMap(item, &build); err != nil {
		return nil, err
	}

	build.SetStatus()
	return &build, nil
}

type BuildStatuses []*BuildStatus

func (b *BuildStatuses) ToJSON() (*bytes.Buffer, error) {
	return toJSON(b)
}

func NewBuildStatuses(items []map[string]*dynamodb.AttributeValue) (BuildStatuses, error) {
	var builds BuildStatuses

	if items == nil {
		return nil, ErrBuildListEmpty
	}

	for _, item := range items {
		build, err := NewBuildStatus(item)
		if err != nil {
			return nil, err
		}

		builds = append(builds, build)
	}

	if len(builds) == 0 {
		return nil, ErrBuildListEmpty
	}

	return builds, nil
}

type BuildPhase struct {
	Name  BuildPhaseLabel
	Phase *BuildPhaseDetail
}

func (b *BuildStatus) NamedPhases() [6]*BuildPhase {
	return [6]*BuildPhase{
		{Name: BuildPhaseName, Phase: b.Build},
		{Name: TestPhaseName, Phase: b.Test},
		{Name: FinalizePhaseName, Phase: b.Finalize},
		{Name: ReleasePhaseName, Phase: b.Release},
		{Name: PostdeployPhaseName, Phase: b.Postdeploy},
		{Name: DeployPhaseName, Phase: b.Deploy},
	}
}

func (b *BuildStatus) NamedPhasesReversed() [6]*BuildPhase {
	return [6]*BuildPhase{
		{Name: DeployPhaseName, Phase: b.Deploy},
		{Name: PostdeployPhaseName, Phase: b.Postdeploy},
		{Name: ReleasePhaseName, Phase: b.Release},
		{Name: FinalizePhaseName, Phase: b.Finalize},
		{Name: TestPhaseName, Phase: b.Test},
		{Name: BuildPhaseName, Phase: b.Build},
	}
}

func (b *BuildStatus) PhaseByName(name BuildPhaseLabel) (*BuildPhaseDetail, error) {
	switch name {
	case BuildPhaseName:
		return b.Build, nil
	case TestPhaseName:
		return b.Test, nil
	case FinalizePhaseName:
		return b.Finalize, nil
	case ReleasePhaseName:
		return b.Release, nil
	case PostdeployPhaseName:
		return b.Postdeploy, nil
	case DeployPhaseName:
		return b.Deploy, nil
	default:
		return nil, fmt.Errorf("%s %w", name, ErrUnknownPhase)
	}
}

func (b *BuildStatus) CurrentPhase() *BuildPhase {
	for _, p := range b.NamedPhases() {
		if p.Phase == nil {
			continue
		}
		if p.Phase.State == PhaseInProgress {
			return p
		}
	}
	return nil
}

// NextActivePhase finds the next phase which already ran or is in progress
func (b *BuildStatus) NextActivePhase(lastPhase *BuildPhase) *BuildPhase {
	// iterate over phases until `lastPhase` is found, then return the next phase which has started
	found := false
	for _, p := range b.NamedPhases() {
		if p.Phase == nil {
			continue
		}
		if found && p.Phase.Start != 0 {
			return p
		}
		if !found && p.Name == lastPhase.Name {
			found = true
		}
	}
	if b.Deploy != nil && b.Deploy.End != 0 {
		return nil
	}
	return lastPhase
}

// FinalPhase returns the last phase which completed or an error if the build is still in progress
func (b *BuildStatus) FinalPhase() (*BuildPhase, error) {
	for _, p := range b.NamedPhasesReversed() {
		if p.Phase == nil {
			continue
		}
		if p.Phase.State == PhaseInProgress {
			return nil, fmt.Errorf("%s phase is still running %w", p.Name, ErrBuildInProgress)
		}
		if p.Phase.State == PhaseSuccess || p.Phase.State == PhaseFailed {
			return p, nil
		}
	}
	return nil, fmt.Errorf("no phases completed")
}

func (b *BuildStatus) FirstFailedPhase() *BuildPhase {
	for _, p := range b.NamedPhases() {
		if p.Phase != nil && p.Phase.State == PhaseFailed {
			return p
		}
	}
	return nil
}

// GetCommitLog retrieves commit.txt stored in S3
func (b *BuildStatus) GetCommitLog(sess *session.Session) (*string, error) {
	if b.Build.Logs == "" || !strings.HasPrefix(b.Build.Logs, "s3://") {
		return nil, ErrBuildLogsNotAvailable
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
