package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/apppackio/apppack/auth"
	apppackaws "github.com/apppackio/apppack/aws"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/codebuild"
	codebuildetypes "github.com/aws/aws-sdk-go-v2/service/codebuild/types"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	dynamodbtypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	ecstypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	ssmtypes "github.com/aws/aws-sdk-go-v2/service/ssm/types"
	sessionManagerPluginSession "github.com/aws/session-manager-plugin/src/sessionmanagerplugin/session"
	// Imported for side effects: registers port and shell session types with session manager plugin
	_ "github.com/aws/session-manager-plugin/src/sessionmanagerplugin/session/portsession"
	_ "github.com/aws/session-manager-plugin/src/sessionmanagerplugin/session/shellsession"
	"github.com/aws/smithy-go"
	"github.com/sirupsen/logrus"
)

const maxEcsDescribeTaskCount = 100

var (
	maxLifetime    = 12 * 60 * 60
	waitForConnect = 60
)

var ShellBackgroundCommand = []string{
	strings.Join([]string{
		"STOP=$(($(date +%s)+" + strconv.Itoa(maxLifetime) + "))",
		// Give user time to connect
		"sleep " + strconv.Itoa(waitForConnect),
		// As long as a user has a shell open, this task will keep running
		"while true",
		"do test -z \"$(pgrep -f ssm-session-worker\\ ecs-execute-command)\" && exit",
		// Timeout if exceeds max lifetime
		"test \"$STOP\" -lt \"$(date +%s)\" && exit 1",
		"sleep 30",
		"done",
	}, "; "),
}

// App is a representation of a AppPack app
type App struct {
	Name                  string
	Pipeline              bool
	ReviewApp             *string
	Session               aws.Config
	Settings              *Settings
	ECSConfig             *ECSConfig
	DeployStatus          *DeployStatus
	PendingDeployStatuses []*DeployStatus
	AWS                   apppackaws.Interface
}

// ReviewApp is a representation of a AppPack review app
type ReviewApp struct {
	PullRequest string `json:"pull_request"`
	Status      string `json:"status"`
	Branch      string `json:"branch"`
	Title       string `json:"title"`
	URL         string `json:"url"`
}
type settingsItem struct {
	PrimaryID   string   `dynamodbav:"primary_id"   json:"primary_id"`
	SecondaryID string   `dynamodbav:"secondary_id" json:"secondary_id"`
	Settings    Settings `dynamodbav:"value"        json:"value"`
}

type Settings struct {
	Cluster struct {
		ARN  string `dynamodbav:"arn"  json:"arn"`
		Name string `dynamodbav:"name" json:"name"`
	} `dynamodbav:"cluster" json:"cluster"`
	LoadBalancer struct {
		ARN    string `dynamodbav:"arn"    json:"arn"`
		Suffix string `dynamodbav:"suffix" json:"suffix"`
	} `dynamodbav:"load_balancer" json:"load_balancer"`
	TargetGroup struct {
		ARN    string `dynamodbav:"arn"    json:"arn"`
		Suffix string `dynamodbav:"suffix" json:"suffix"`
	} `dynamodbav:"target_group" json:"target_group"`
	Domains []string `dynamodbav:"domains" json:"domains"`
	Shell   struct {
		Command    string `dynamodbav:"command"     json:"command"`
		TaskFamily string `dynamodbav:"task_family" json:"task_family"`
	} `dynamodbav:"shell" json:"shell"`
	DBUtils struct {
		ShellTaskFamily    string `dynamodbav:"shell_task_family"    json:"shell_task_family"`
		S3Bucket           string `dynamodbav:"s3_bucket"            json:"s3_bucket"`
		DumpLoadTaskFamily string `dynamodbav:"dumpload_task_family" json:"dumpload_task_family"`
		Engine             string `dynamodbav:"engine"               json:"engine"`
	} `dynamodbav:"dbutils" json:"dbutils"`
	CodebuildProject struct {
		Name string `dynamodbav:"name" json:"name"`
	} `dynamodbav:"codebuild_project" json:"codebuild_project"`
	LogGroup struct {
		Name string `dynamodbav:"name" json:"name"`
	} `dynamodbav:"log_group" json:"log_group"`
	StackID string `dynamodbav:"stack_id" json:"stack_id"`
}

type deployStatusItem struct {
	PrimaryID    string       `dynamodbav:"primary_id"   json:"primary_id"`
	SecondaryID  string       `dynamodbav:"secondary_id" json:"secondary_id"`
	DeployStatus DeployStatus `dynamodbav:"value"        json:"value"`
}

type DeployStatus struct {
	Phase       string    `dynamodbav:"phase"        json:"phase"`
	Processes   []Process `dynamodbav:"processes"    json:"processes"`
	BuildID     string    `dynamodbav:"build_id"     json:"build_id"`
	LastUpdate  int64     `dynamodbav:"last_update"  json:"last_update"`
	Commit      string    `dynamodbav:"commit"       json:"commit"`
	BuildNumber int       `dynamodbav:"build_number" json:"build_number"`
	Failed      bool      `dynamodbav:"failed"       json:"failed"`
}

func (d *DeployStatus) FindProcess(name string) (*Process, error) {
	if d == nil {
		return nil, fmt.Errorf("process '%s' not found", name)
	}

	for _, p := range d.Processes {
		if p.Name == name {
			return &p, nil
		}
	}

	return nil, fmt.Errorf("process '%s' not found", name)
}

type Process struct {
	Name         string `dynamodbav:"name"          json:"name"`
	CPU          string `dynamodbav:"cpu"           json:"cpu"`
	Memory       string `dynamodbav:"memory"        json:"memory"`
	MinProcesses int    `dynamodbav:"min_processes" json:"min_processes"`
	MaxProcesses int    `dynamodbav:"max_processes" json:"max_processes"`
	State        string `dynamodbav:"state"         json:"state"`
	Command      string `dynamodbav:"command"       json:"command"`
}

// locationName is the tag used by aws-sdk internally
// we can use it to load specific AWS Input types from our JSON
type ecsConfigItem struct {
	PrimaryID   string    `locationName:"primary_id"`
	SecondaryID string    `locationName:"secondary_id"`
	ECSConfig   ECSConfig `locationName:"value"`
}

type ECSConfig struct {
	RunTaskArgs        ecs.RunTaskInput                `locationName:"run_task_args"`
	RunTaskArgsFargate ecs.RunTaskInput                `locationName:"run_task_args_fargate"`
	TaskDefinitionArgs ecs.RegisterTaskDefinitionInput `locationName:"task_definition_args"`
}

type ECSSizeConfiguration struct {
	CPU    int
	Memory int
}

var (
	QuarterCPU = 256
	HalfCPU    = 512
	FullCPU    = 1024
	OneGB      = 1024
)

var FargateSupportedConfigurations = []ECSSizeConfiguration{
	{CPU: QuarterCPU, Memory: OneGB / 2},
	{CPU: QuarterCPU, Memory: OneGB},
	{CPU: QuarterCPU, Memory: 2 * OneGB},
	{CPU: HalfCPU, Memory: OneGB},
	{CPU: HalfCPU, Memory: 2 * OneGB},
	{CPU: HalfCPU, Memory: 3 * OneGB},
	{CPU: HalfCPU, Memory: 4 * OneGB},
	{CPU: FullCPU, Memory: 2 * OneGB},
	{CPU: FullCPU, Memory: 3 * OneGB},
	{CPU: FullCPU, Memory: 4 * OneGB},
	{CPU: FullCPU, Memory: 5 * OneGB},
	{CPU: FullCPU, Memory: 6 * OneGB},
	{CPU: FullCPU, Memory: 7 * OneGB},
	{CPU: FullCPU, Memory: 8 * OneGB},
	{CPU: 2 * FullCPU, Memory: 4 * OneGB},
	{CPU: 2 * FullCPU, Memory: 5 * OneGB},
	{CPU: 2 * FullCPU, Memory: 6 * OneGB},
	{CPU: 2 * FullCPU, Memory: 7 * OneGB},
	{CPU: 2 * FullCPU, Memory: 8 * OneGB},
	{CPU: 2 * FullCPU, Memory: 9 * OneGB},
	{CPU: 2 * FullCPU, Memory: 10 * OneGB},
	{CPU: 2 * FullCPU, Memory: 11 * OneGB},
	{CPU: 2 * FullCPU, Memory: 12 * OneGB},
	{CPU: 2 * FullCPU, Memory: 13 * OneGB},
	{CPU: 2 * FullCPU, Memory: 14 * OneGB},
	{CPU: 2 * FullCPU, Memory: 15 * OneGB},
	{CPU: 2 * FullCPU, Memory: 16 * OneGB},
	{CPU: 4 * FullCPU, Memory: 8 * OneGB},
	{CPU: 4 * FullCPU, Memory: 9 * OneGB},
	{CPU: 4 * FullCPU, Memory: 10 * OneGB},
	{CPU: 4 * FullCPU, Memory: 11 * OneGB},
	{CPU: 4 * FullCPU, Memory: 12 * OneGB},
	{CPU: 4 * FullCPU, Memory: 13 * OneGB},
	{CPU: 4 * FullCPU, Memory: 14 * OneGB},
	{CPU: 4 * FullCPU, Memory: 15 * OneGB},
	{CPU: 4 * FullCPU, Memory: 16 * OneGB},
	{CPU: 4 * FullCPU, Memory: 17 * OneGB},
	{CPU: 4 * FullCPU, Memory: 18 * OneGB},
	{CPU: 4 * FullCPU, Memory: 19 * OneGB},
	{CPU: 4 * FullCPU, Memory: 20 * OneGB},
	{CPU: 4 * FullCPU, Memory: 21 * OneGB},
	{CPU: 4 * FullCPU, Memory: 22 * OneGB},
	{CPU: 4 * FullCPU, Memory: 23 * OneGB},
	{CPU: 4 * FullCPU, Memory: 24 * OneGB},
	{CPU: 4 * FullCPU, Memory: 25 * OneGB},
	{CPU: 4 * FullCPU, Memory: 26 * OneGB},
	{CPU: 4 * FullCPU, Memory: 27 * OneGB},
	{CPU: 4 * FullCPU, Memory: 28 * OneGB},
	{CPU: 4 * FullCPU, Memory: 29 * OneGB},
	{CPU: 4 * FullCPU, Memory: 30 * OneGB},
	{CPU: 8 * FullCPU, Memory: 16 * OneGB},
	{CPU: 8 * FullCPU, Memory: 20 * OneGB},
	{CPU: 8 * FullCPU, Memory: 24 * OneGB},
	{CPU: 8 * FullCPU, Memory: 28 * OneGB},
	{CPU: 8 * FullCPU, Memory: 32 * OneGB},
	{CPU: 8 * FullCPU, Memory: 36 * OneGB},
	{CPU: 8 * FullCPU, Memory: 40 * OneGB},
	{CPU: 8 * FullCPU, Memory: 44 * OneGB},
	{CPU: 8 * FullCPU, Memory: 48 * OneGB},
	{CPU: 8 * FullCPU, Memory: 52 * OneGB},
	{CPU: 8 * FullCPU, Memory: 56 * OneGB},
	{CPU: 8 * FullCPU, Memory: 60 * OneGB},
	{CPU: 16 * FullCPU, Memory: 32 * OneGB},
	{CPU: 16 * FullCPU, Memory: 40 * OneGB},
	{CPU: 16 * FullCPU, Memory: 48 * OneGB},
	{CPU: 16 * FullCPU, Memory: 56 * OneGB},
	{CPU: 16 * FullCPU, Memory: 64 * OneGB},
	{CPU: 16 * FullCPU, Memory: 72 * OneGB},
	{CPU: 16 * FullCPU, Memory: 80 * OneGB},
	{CPU: 16 * FullCPU, Memory: 88 * OneGB},
	{CPU: 16 * FullCPU, Memory: 96 * OneGB},
	{CPU: 16 * FullCPU, Memory: 104 * OneGB},
	{CPU: 16 * FullCPU, Memory: 112 * OneGB},
	{CPU: 16 * FullCPU, Memory: 120 * OneGB},
	{CPU: 16 * FullCPU, Memory: 128 * OneGB},
	{CPU: 16 * FullCPU, Memory: 136 * OneGB},
	{CPU: 16 * FullCPU, Memory: 144 * OneGB},
	{CPU: 16 * FullCPU, Memory: 152 * OneGB},
	{CPU: 16 * FullCPU, Memory: 160 * OneGB},
}

func (a *App) IsReviewApp() bool {
	return a.ReviewApp != nil
}

func (a *App) IsFargate() (bool, error) {
	err := a.LoadECSConfig()
	if err != nil {
		return false, err
	}

	return a.ECSConfig.RunTaskArgs.LaunchType == "FARGATE", nil
}

func (a *App) ValidateECSTaskSize(size ECSSizeConfiguration) error {
	fargate, err := a.IsFargate()
	if err != nil {
		return err
	}

	if fargate {
		logrus.Debug("fargate task detected")

		for _, supported := range FargateSupportedConfigurations {
			if supported.CPU == size.CPU && supported.Memory == size.Memory {
				return nil
			}
		}
	} else if size.CPU >= 128 && size.CPU <= 10240 {
		return nil
	}

	return errors.New("unsupported cpu/memory configuration -- see https://docs.aws.amazon.com/AmazonECS/latest/developerguide/task-cpu-memory-error.html")
}

func (a *App) ReviewAppSettings() (*Settings, error) {
	if !a.IsReviewApp() {
		return nil, errors.New("only review apps have review app settings")
	}

	Item, err := ddbItem(a.Session, fmt.Sprintf("APP#%s:%s", a.Name, *a.ReviewApp), "settings")
	if err != nil {
		return nil, err
	}

	i := settingsItem{}

	err = attributevalue.UnmarshalMap(*Item, &i)
	if err != nil {
		return nil, err
	}

	return &i.Settings, nil
}

// ServiceName gets the name of a service for the app taking into account review apps
func (a *App) ServiceName(service string) string {
	if a.IsReviewApp() {
		return fmt.Sprintf("%s-pr%s-%s", a.Name, *a.ReviewApp, service)
	}

	return fmt.Sprintf("%s-%s", a.Name, service)
}

// TaskDefinition gets the Task Definition for a specific task type
func (a *App) TaskDefinition(name string) (*ecstypes.TaskDefinition, []ecstypes.Tag, error) {
	family := a.ServiceName(name)
	ecsSvc := ecs.NewFromConfig(a.Session)
	// verify task exists
	task, err := ecsSvc.DescribeTaskDefinition(context.Background(), &ecs.DescribeTaskDefinitionInput{
		TaskDefinition: &family,
		Include:        []ecstypes.TaskDefinitionField{ecstypes.TaskDefinitionFieldTags},
	})
	if err != nil {
		return nil, nil, err
	}

	return task.TaskDefinition, task.Tags, nil
}

func buildSystemFromTaskTags(tags []ecstypes.Tag) *string {
	for _, tag := range tags {
		if *tag.Key == "apppack:buildSystem" {
			return tag.Value
		}
	}

	emptyString := ""
	return &emptyString
}

func (a *App) ShellTaskFamily() (*string, *string, error) {
	taskDefn, tags, err := a.TaskDefinition("shell")

	buildSystem := buildSystemFromTaskTags(tags)

	if err != nil {
		return nil, nil, err
	}

	return taskDefn.Family, buildSystem, nil
}

// URL is used to lookup the app url from settings
// pipelines need to do this for their review apps so it is passed in as an argument
func (a *App) URL(reviewApp *string) (*string, error) {
	var settings *Settings

	var err error

	switch {
	case reviewApp != nil:
		a.ReviewApp = reviewApp

		settings, err = a.ReviewAppSettings()
		if err != nil {
			return nil, err
		}

		a.ReviewApp = nil
	case a.IsReviewApp():
		settings, err = a.ReviewAppSettings()
		if err != nil {
			return nil, err
		}
	default:
		err := a.LoadSettings()
		if err != nil {
			return nil, err
		}

		settings = a.Settings
	}

	return aws.String("https://" + settings.Domains[0]), nil
}

func (a *App) GetReviewApps() ([]*ReviewApp, error) {
	if !a.Pipeline {
		return nil, fmt.Errorf("%s is not a pipeline and cannot have review apps", a.Name)
	}

	parameters, err := SsmParameters(a.Session, fmt.Sprintf("/apppack/pipelines/%s/review-apps/pr/", a.Name))
	if err != nil {
		return nil, err
	}

	var reviewApps []*ReviewApp

	for _, parameter := range parameters {
		r := ReviewApp{}

		err = json.Unmarshal([]byte(*parameter.Value), &r)
		if err != nil {
			return nil, err
		}

		reviewApps = append(reviewApps, &r)
	}

	return reviewApps, nil
}

func (a *App) ReviewAppExists() (bool, error) {
	if !a.Pipeline {
		return false, fmt.Errorf("%s is not a pipeline and cannot have review apps", a.Name)
	}

	parameter, err := SsmParameter(a.Session, fmt.Sprintf("/apppack/pipelines/%s/review-apps/pr/%s", a.Name, *a.ReviewApp))
	if err != nil {
		return false, fmt.Errorf("ReviewApp named %s:%s does not exist", a.Name, *a.ReviewApp)
	}

	r := ReviewApp{}

	err = json.Unmarshal([]byte(*parameter.Value), &r)
	if err != nil {
		return false, err
	}

	if r.Status != "created" {
		return false, errors.New("ReviewApp isn't created")
	}

	return true, nil
}

func (a *App) ddbItem(key string) (*map[string]dynamodbtypes.AttributeValue, error) {
	if !a.IsReviewApp() {
		return ddbItem(a.Session, "APP#"+a.Name, key)
	}
	// TODO: move DEPLOYSTATUS to standard review app location
	if strings.HasPrefix(key, "CONFIG") || key == "settings" || strings.HasPrefix(key, "DEPLOYSTATUS") {
		return ddbItem(a.Session, "APP#"+a.Name, key)
	}
	// review apps are at APP#{appname}:{pr}
	return ddbItem(a.Session, fmt.Sprintf("APP#%s:%s", a.Name, *a.ReviewApp), key)
}

// LoadECSConfig will set the app.ECSConfig value from DDB
func (a *App) LoadECSConfig() error {
	if a.ECSConfig != nil {
		return nil
	}

	Item, err := a.ddbItem("CONFIG#ecs")
	if err != nil {
		return err
	}

	i := ecsConfigItem{}

	decoder := attributevalue.NewDecoder(func(d *attributevalue.DecoderOptions) {
		d.TagKey = "locationName"
	})

	err = decoder.Decode(&dynamodbtypes.AttributeValueMemberM{Value: *Item}, &i)
	if err != nil {
		return err
	}

	a.ECSConfig = &i.ECSConfig

	return nil
}

// GetDeployStatus will get a DeployStatus value from DDB
func (a *App) GetDeployStatus(buildARN string) (*DeployStatus, error) {
	key := "DEPLOYSTATUS"
	if a.IsReviewApp() {
		key = key + "#" + *a.ReviewApp
	}

	if buildARN != "" {
		key = key + "#" + buildARN
	}

	Item, err := a.ddbItem(key)
	if err != nil {
		return nil, err
	}

	i := deployStatusItem{}

	err = attributevalue.UnmarshalMap(*Item, &i)
	if err != nil {
		return nil, err
	}

	return &i.DeployStatus, nil
}

// GetServices will get a list of current services from the deploy status
func (a *App) GetServices() ([]string, error) {
	err := a.LoadDeployStatus()
	if err != nil {
		return nil, err
	}

	var services []string

	for _, process := range a.DeployStatus.Processes {
		services = append(services, process.Name)
	}

	return services, nil
}

// LoadDeployStatus will get the app.DeployStatus value from DDB
func (a *App) LoadDeployStatus() error {
	if a.DeployStatus != nil {
		return nil
	}

	deployStatus, err := a.GetDeployStatus("")
	if err != nil {
		return err
	}

	a.DeployStatus = deployStatus

	return nil
}

// LoadSettings will set the app.Settings value from DDB
func (a *App) LoadSettings() error {
	if a.Settings != nil {
		return nil
	}

	Item, err := a.ddbItem("settings")
	if err != nil {
		return err
	}

	i := settingsItem{}

	err = attributevalue.UnmarshalMap(*Item, &i)
	if err != nil {
		return err
	}

	a.Settings = &i.Settings

	return nil
}

// StartTask start a new task on ECS
func (a *App) StartTask(taskFamily *string, command []string, taskOverride *ecstypes.TaskOverride, fargate bool) (*ecstypes.Task, error) {
	ecsSvc := ecs.NewFromConfig(a.Session)

	err := a.LoadSettings()
	if err != nil {
		return nil, err
	}

	err = a.LoadECSConfig()
	if err != nil {
		return nil, err
	}

	taskDefn, err := ecsSvc.DescribeTaskDefinition(context.Background(), &ecs.DescribeTaskDefinitionInput{
		TaskDefinition: taskFamily,
	})
	if err != nil {
		return nil, err
	}

	var runTaskArgs ecs.RunTaskInput
	if fargate {
		runTaskArgs = a.ECSConfig.RunTaskArgsFargate
	} else {
		runTaskArgs = a.ECSConfig.RunTaskArgs
	}

	var cmd []string
	for i := range command {
		cmd = append(cmd, command[i])
	}

	email, err := auth.WhoAmI()
	if err != nil {
		return nil, err
	}

	startedBy := "apppack-cli/shell/" + *email
	runTaskArgs.TaskDefinition = taskDefn.TaskDefinition.TaskDefinitionArn
	runTaskArgs.StartedBy = &startedBy

	memory := int32(0)
	if taskOverride.Memory != nil {
		memoryInt64, err := strconv.ParseInt(*taskOverride.Memory, 10, 32)
		if err != nil {
			return nil, err
		}
		memory = int32(memoryInt64)
	}

	if len(taskOverride.ContainerOverrides) == 0 {
		taskOverride.ContainerOverrides = []ecstypes.ContainerOverride{{}}
	}

	taskOverride.ContainerOverrides[0].Name = taskDefn.TaskDefinition.ContainerDefinitions[0].Name
	taskOverride.ContainerOverrides[0].Command = cmd

	if memory > 0 {
		taskOverride.ContainerOverrides[0].Memory = aws.Int32(memory)
	}

	runTaskArgs.Overrides = taskOverride

	ecsTaskOutput, err := ecsSvc.RunTask(context.Background(), &runTaskArgs)
	if err != nil {
		return nil, err
	}

	if len(ecsTaskOutput.Failures) > 0 {
		return nil, fmt.Errorf("RunTask failure: %v", ecsTaskOutput.Failures)
	}

	return &ecsTaskOutput.Tasks[0], nil
}

// WaitForTaskStopped waits for a task to be stopped
func (a *App) WaitForTaskStopped(task *ecstypes.Task) (*int32, error) {
	ecsSvc := ecs.NewFromConfig(a.Session)
	input := ecs.DescribeTasksInput{
		Cluster: task.ClusterArn,
		Tasks:   []string{*task.TaskArn},
	}
	// MaxSessionDurationSeconds is 3600. This will wait _almost_ that long
	waiter := ecs.NewTasksStoppedWaiter(ecsSvc, func(o *ecs.TasksStoppedWaiterOptions) {
		o.MaxDelay = 6 * time.Second
		o.MinDelay = 6 * time.Second
	})
	err := waiter.Wait(context.Background(), &input, 3570*time.Second)
	if err != nil {
		return nil, err
	}

	taskDesc, err := ecsSvc.DescribeTasks(context.Background(), &input)
	if err != nil {
		return nil, err
	}

	stoppedTask := taskDesc.Tasks[0]
	if stoppedTask.StopCode != "EssentialContainerExited" {
		return nil, fmt.Errorf("task %s failed %s: %s", *stoppedTask.TaskArn, string(stoppedTask.StopCode), *stoppedTask.StoppedReason)
	}

	return stoppedTask.Containers[0].ExitCode, nil
}

func (a *App) CreateEcsSession(task *ecstypes.Task, shellCmd string) (*ecstypes.Session, error) {
	ecsSvc := ecs.NewFromConfig(a.Session)

	err := a.LoadSettings()
	if err != nil {
		return nil, err
	}

	execCmdInput := ecs.ExecuteCommandInput{
		Cluster:     task.ClusterArn,
		Command:     &shellCmd,
		Container:   task.Containers[0].Name,
		Interactive: true,
		Task:        task.TaskArn,
	}
	retries := 20
	// it takes some time for the SSM agent to startup
	// poll for availability
	for retries > 0 {
		time.Sleep(2 * time.Second)

		out, err := ecsSvc.ExecuteCommand(context.Background(), &execCmdInput)

		var apiErr smithy.APIError

		if err == nil {
			return out.Session, nil
		} else if errors.As(err, &apiErr) {
			if apiErr.ErrorCode() != "InvalidParameterException" {
				return nil, err
			}
		} else {
			return nil, err
		}

		retries--
	}

	return nil, errors.New("timeout attempting to connect to SSM Agent")
}

// ConnectToEcsSession open a SSM Session to the Docker host and exec into container
func (a *App) ConnectToEcsSession(ecsSession *ecstypes.Session) error {
	region := a.Session.Region

	arg1, err := json.Marshal(ecsSession)
	if err != nil {
		return err
	}

	args := []string{
		"session-manager-plugin",
		string(arg1),
		region,
		"StartSession",
	}
	// Ignore Ctrl+C to keep the session active;
	// reset the signal afterward so the main function
	// can handle interrupts during the rest of the program's execution.
	signal.Ignore(syscall.SIGINT)
	defer signal.Reset(syscall.SIGINT)
	sessionManagerPluginSession.ValidateInputAndStartSession(args, os.Stdout)

	return nil
}

// StartBuild starts a new CodeBuild run
func (a *App) StartBuild(createReviewApp bool) (*codebuildetypes.Build, error) {
	codebuildSvc := codebuild.NewFromConfig(a.Session)

	err := a.LoadSettings()
	if err != nil {
		return nil, err
	}

	buildInput := codebuild.StartBuildInput{
		ProjectName: &a.Settings.CodebuildProject.Name,
	}
	if a.IsReviewApp() {
		buildInput.SourceVersion = aws.String("pr/" + *a.ReviewApp)
		if createReviewApp {
			buildInput.EnvironmentVariablesOverride = []codebuildetypes.EnvironmentVariable{
				{
					Name:  aws.String("REVIEW_APP_STATUS"),
					Value: aws.String("created"),
					Type:  codebuildetypes.EnvironmentVariableTypePlaintext,
				},
			}
		}
	}

	build, err := codebuildSvc.StartBuild(context.Background(), &buildInput)
	if err != nil {
		return nil, err
	}
	return build.Build, nil
}

// ListBuilds lists recent CodeBuild runs
func (a *App) RecentBuilds(count int) ([]BuildStatus, error) {
	ddbSvc := dynamodb.NewFromConfig(a.Session)

	primaryID := "APP#" + a.Name
	if a.IsReviewApp() {
		primaryID = fmt.Sprintf("%s:%s", primaryID, *a.ReviewApp)
	}

	logrus.WithFields(logrus.Fields{"count": count}).Debug("fetching build list from DDB")

	ddbResp, err := ddbSvc.Query(context.Background(), &dynamodb.QueryInput{
		TableName:              aws.String("apppack"),
		KeyConditionExpression: aws.String("primary_id = :id1  AND begins_with(secondary_id,:id2)"),
		ExpressionAttributeValues: map[string]dynamodbtypes.AttributeValue{
			":id1": &dynamodbtypes.AttributeValueMemberS{Value: primaryID},
			":id2": &dynamodbtypes.AttributeValueMemberS{Value: "BUILD#"},
		},

		Limit:            aws.Int32(int32(count)),
		ScanIndexForward: aws.Bool(false),
	})
	if err != nil {
		return nil, err
	}

	if ddbResp.Items == nil {
		return nil, errors.New("could not find any builds")
	}

	var i []BuildStatus

	err = attributevalue.UnmarshalListOfMaps(ddbResp.Items, &i)
	if err != nil {
		return nil, err
	}

	if len(i) == 0 {
		return nil, errors.New("could not find any builds")
	}

	return i, nil
}

// GetBuildStatus retrieves a build from the buildNumber
// if buildNumber is -1, the most recent build will be retrieved
func (a *App) GetBuildStatus(buildNumber int) (*BuildStatus, error) {
	var build BuildStatus

	if buildNumber == -1 {
		builds, err := a.RecentBuilds(1)
		if err != nil {
			return nil, err
		}

		build = builds[0]
	} else {
		item, err := a.ddbItem(fmt.Sprintf("BUILD#%010d", buildNumber))
		if err != nil {
			return nil, err
		}

		err = attributevalue.UnmarshalMap(*item, &build)
		if err != nil {
			return nil, err
		}

		if len(build.Build.Arns) == 0 {
			return nil, errors.New("build has not started yet -- try again in a few seconds")
		}
	}

	return &build, nil
}

// ConfigPrefix returns the SSM Parameter Store prefix for config variables
func (a *App) ConfigPrefix() string {
	if a.IsReviewApp() {
		return fmt.Sprintf("/apppack/pipelines/%s/review-apps/pr/%s/config/", a.Name, *a.ReviewApp)
	} else if a.Pipeline {
		return fmt.Sprintf("/apppack/pipelines/%s/config/", a.Name)
	}

	return fmt.Sprintf("/apppack/apps/%s/config/", a.Name)
}

// GetConfig returns a list of config parameters for the app
func (a *App) GetConfig() (ConfigVariables, error) {
	prefix := a.ConfigPrefix()

	parameters, err := SsmParameters(a.Session, prefix)
	if err != nil {
		return nil, err
	}

	return NewConfigVariables(parameters), nil
}

// GetConfigWithManaged returns a list of config parameters for the app with managed value populated
func (a *App) GetConfigWithManaged() (ConfigVariables, error) {
	configVars, err := a.GetConfig()
	if err != nil {
		return nil, err
	}

	ssmSvc := ssm.NewFromConfig(a.Session)

	err = configVars.Transform(func(v *ConfigVariable) error {
		return v.LoadManaged(func(input *ssm.ListTagsForResourceInput) (*ssm.ListTagsForResourceOutput, error) {
			return ssmSvc.ListTagsForResource(context.Background(), input)
		})
	})
	if err != nil {
		return nil, err
	}

	return configVars, nil
}

// SetConfig sets a config value for the app
func (a *App) SetConfig(key, value string, overwrite bool) error {
	parameterName := fmt.Sprintf("%s%s", a.ConfigPrefix(), key)
	ssmSvc := ssm.NewFromConfig(a.Session)
	parameterType := ssmtypes.ParameterTypeSecureString
	_, err := ssmSvc.PutParameter(context.Background(), &ssm.PutParameterInput{
		Name:      &parameterName,
		Type:      parameterType,
		Overwrite: &overwrite,
		Value:     &value,
	})

	return err
}

// GetConsoleURL generate a URL which will sign the user in to the AWS console and redirect to the desinationURL
func (a *App) GetConsoleURL(destinationURL string) (*string, error) {
	return auth.GetConsoleURL(a.Session, destinationURL)
}

// DescribeTasks generate a URL which will sign the user in to the AWS console and redirect to the desinationURL
func (a *App) DescribeTasks() ([]ecstypes.Task, error) {
	err := a.LoadSettings()
	if err != nil {
		return nil, err
	}

	ecsSvc := ecs.NewFromConfig(a.Session)
	chunkedTaskARNs := [][]string{{}}
	input := ecs.ListTasksInput{
		Cluster: &a.Settings.Cluster.ARN,
	}

	logrus.WithFields(logrus.Fields{"cluster": a.Settings.Cluster.ARN}).Debug("fetching task list")

	// handle chunking logic
	addTaskARNToChunk := func(taskARN string) {
		if len(chunkedTaskARNs[len(chunkedTaskARNs)-1]) >= maxEcsDescribeTaskCount {
			chunkedTaskARNs = append(chunkedTaskARNs, []string{})
		}

		chunkedTaskARNs[len(chunkedTaskARNs)-1] = append(chunkedTaskARNs[len(chunkedTaskARNs)-1], taskARN)
	}

	paginator := ecs.NewListTasksPaginator(ecsSvc, &input)
	for paginator.HasMorePages() {
		resp, err := paginator.NextPage(context.Background())
		if err != nil {
			return nil, err
		}

		for _, taskARN := range resp.TaskArns {
			addTaskARNToChunk(taskARN)
		}
	}

	var describedTasks []ecstypes.Task

	for i := range chunkedTaskARNs {
		logrus.WithFields(logrus.Fields{"count": len(chunkedTaskARNs[i])}).Debug("fetching task descriptions")

		describeTasksOutput, err := ecsSvc.DescribeTasks(context.Background(), &ecs.DescribeTasksInput{
			Tasks:   chunkedTaskARNs[i],
			Cluster: &a.Settings.Cluster.ARN,
			Include: []ecstypes.TaskField{ecstypes.TaskFieldTags},
		})
		if err != nil {
			return nil, err
		}

		describedTasks = append(describedTasks, describeTasksOutput.Tasks...)
	}

	var appTasks []ecstypes.Task

	for _, task := range describedTasks {
		isApp := false
		isReviewApp := false

		for _, t := range task.Tags {
			if *t.Key == "apppack:appName" && *t.Value == a.Name {
				isApp = true
			}

			if a.IsReviewApp() {
				if *t.Key == "apppack:reviewApp" && *t.Value == "pr/"+*a.ReviewApp {
					isReviewApp = true
				}
			}
		}

		if isApp {
			if a.IsReviewApp() {
				if isReviewApp {
					appTasks = append(appTasks, task)
				}
			} else {
				appTasks = append(appTasks, task)
			}
		}
	}

	return appTasks, nil
}

func (a *App) GetECSEvents(service string) ([]ecstypes.ServiceEvent, error) {
	ecsSvc := ecs.NewFromConfig(a.Session)

	if err := a.LoadSettings(); err != nil {
		return nil, err
	}

	logrus.WithFields(logrus.Fields{"service": service}).Debug("fetching service events")

	serviceStatus, err := ecsSvc.DescribeServices(context.Background(), &ecs.DescribeServicesInput{
		Cluster:  &a.Settings.Cluster.ARN,
		Services: []string{a.ServiceName(service)},
	})
	if err != nil {
		return nil, err
	}

	if len(serviceStatus.Services) == 0 {
		return nil, fmt.Errorf("could not find service %s", service)
	}

	events := serviceStatus.Services[0].Events
	// reverse events so the oldest is first
	for i, j := 0, len(events)-1; i < j; i, j = i+1, j-1 {
		events[i], events[j] = events[j], events[i]
	}

	return events, nil
}

func (a *App) DBDumpLocation(prefix string) (*s3.GetObjectInput, error) {
	currentTime := time.Now()

	username, err := auth.WhoAmI()
	if err != nil {
		return nil, err
	}

	if err = a.LoadSettings(); err != nil {
		return nil, err
	}

	if a.IsReviewApp() {
		prefix = fmt.Sprintf("%spr%s/", prefix, *a.ReviewApp)
	}

	var extension string
	if strings.Contains(a.Settings.DBUtils.Engine, "mysql") {
		extension = "sql.gz"
	} else if strings.Contains(a.Settings.DBUtils.Engine, "postgres") {
		extension = "dump"
	} else {
		return nil, fmt.Errorf("unknown database engine %s", a.Settings.DBUtils.Engine)
	}

	input := s3.GetObjectInput{
		Key:    aws.String(fmt.Sprintf("%s%s-%s.%s", prefix, currentTime.Format("20060102150405"), *username, extension)),
		Bucket: &a.Settings.DBUtils.S3Bucket,
	}

	return &input, nil
}

func (a *App) DBDumpLoadFamily() (*string, error) {
	taskDefn, _, err := a.TaskDefinition("dbutils")
	if err != nil {
		return nil, err
	}

	return taskDefn.Family, nil
}

func (a *App) DBDump() (*ecstypes.Task, *s3.GetObjectInput, error) {
	getObjectInput, err := a.DBDumpLocation("dumps/")
	if err != nil {
		return nil, nil, err
	}

	family, err := a.DBDumpLoadFamily()
	if err != nil {
		return nil, nil, err
	}

	task, err := a.StartTask(
		family,
		[]string{"dump-to-s3.sh", fmt.Sprintf("s3://%s/%s", *getObjectInput.Bucket, *getObjectInput.Key)},
		&ecstypes.TaskOverride{},
		true,
	)
	if err != nil {
		return nil, nil, err
	}

	return task, getObjectInput, nil
}

// DBShellTaskInfo gets the family and command to execute for a db shell task
func (a *App) DBShellTaskInfo() (*string, *string, error) {
	err := a.LoadSettings()
	if err != nil {
		return nil, nil, err
	}

	var exec string

	if strings.Contains(a.Settings.DBUtils.Engine, "mysql") {
		database := a.Name
		if a.IsReviewApp() {
			database = fmt.Sprintf("%s-pr%s", database, *a.ReviewApp)
		}

		exec = "mysql --database=" + database
	} else if strings.Contains(a.Settings.DBUtils.Engine, "postgres") {
		exec = "psql"
	} else {
		return nil, nil, fmt.Errorf("unknown database engine %s", a.Settings.DBUtils.Engine)
	}

	var family string

	if a.IsReviewApp() {
		exec = fmt.Sprintf("%s %s-pr%s", exec, a.Name, *a.ReviewApp)
		family = fmt.Sprintf("%s-pr%s-dbshell", a.Name, *a.ReviewApp)
	} else {
		family = a.Settings.DBUtils.ShellTaskFamily
	}

	return &family, &exec, nil
}

type Scaling struct {
	CPU          int `json:"cpu"`
	Memory       int `json:"memory"`
	MinProcesses int `json:"min_processes"`
	MaxProcesses int `json:"max_processes"`
}

func (a *App) ResizeProcess(processType string, cpu, memory int) error {
	return a.SetScaleParameter(processType, nil, nil, &cpu, &memory)
}

func (a *App) ScaleProcess(processType string, minProcessCount, maxProcessCount int) error {
	return a.SetScaleParameter(processType, &minProcessCount, &maxProcessCount, nil, nil)
}

// SetScaleParameter updates process count and cpu/ram with any non-nil values provided
// if it is not yet set, the defaults from ECSConfig will be used
func (a *App) SetScaleParameter(processType string, minProcessCount, maxProcessCount, cpu, memory *int) error {
	ssmSvc := ssm.NewFromConfig(a.Session)

	var parameterName string
	if a.IsReviewApp() {
		parameterName = fmt.Sprintf("/apppack/pipelines/%s/review-apps/pr/%s/scaling", a.Name, *a.ReviewApp)
	} else if a.Pipeline {
		parameterName = fmt.Sprintf("/apppack/pipelines/%s/scaling", a.Name)
	} else {
		parameterName = fmt.Sprintf("/apppack/apps/%s/scaling", a.Name)
	}

	if a.Pipeline && maxProcessCount != nil && minProcessCount != nil && *maxProcessCount != *minProcessCount {
		return errors.New("auto-scaling is not supported on pipelines")
	}

	parameterOutput, err := ssmSvc.GetParameter(context.Background(), &ssm.GetParameterInput{
		Name: &parameterName,
	})

	var scaling map[string]*Scaling

	if err != nil {
		scaling = map[string]*Scaling{}
	} else if err = json.Unmarshal([]byte(*parameterOutput.Parameter.Value), &scaling); err != nil {
		return err
	}

	_, ok := scaling[processType]
	if !ok {
		if err = a.LoadECSConfig(); err != nil {
			return err
		}

		cpu, err := strconv.Atoi(*a.ECSConfig.TaskDefinitionArgs.Cpu)
		if err != nil {
			return err
		}

		mem, err := strconv.Atoi(*a.ECSConfig.TaskDefinitionArgs.Memory)
		if err != nil {
			return err
		}

		scaling[processType] = &Scaling{
			CPU:          cpu,
			Memory:       mem,
			MinProcesses: 1,
			MaxProcesses: 1,
		}
	}

	if minProcessCount != nil {
		scaling[processType].MinProcesses = *minProcessCount
	}

	if maxProcessCount != nil {
		scaling[processType].MaxProcesses = *maxProcessCount
	}

	if cpu != nil {
		scaling[processType].CPU = *cpu
	}

	if memory != nil {
		scaling[processType].Memory = *memory
	}

	scalingJSON, err := json.Marshal(scaling)
	if err != nil {
		return err
	}

	parameterType := ssmtypes.ParameterTypeString
	_, err = ssmSvc.PutParameter(context.Background(), &ssm.PutParameterInput{
		Name:      &parameterName,
		Type:      parameterType,
		Value:     aws.String(string(scalingJSON)),
		Overwrite: aws.Bool(true),
	})
	if err != nil {
		return err
	}

	return nil
}

// Init will pull in app settings from DyanmoDB and provide helper
func Init(name string, awsCredentials bool, sessionDuration int) (*App, error) {
	var reviewApp *string

	if strings.Contains(name, ":") {
		parts := strings.Split(name, ":")
		name = parts[0]
		reviewApp = &parts[1]
	} else {
		reviewApp = nil
	}

	app := App{
		Name:      name,
		ReviewApp: reviewApp,
	}

	if awsCredentials {
		cfg, err := config.LoadDefaultConfig(context.Background())
		if err != nil {
			return nil, err
		}
		app.Session = cfg
		app.AWS = apppackaws.New(cfg)

		err = app.LoadSettings()
		if err != nil {
			return nil, err
		}
		// this is a horribly hacky way to figure out if the app is a pipeline, but it works
		app.Pipeline = strings.Contains(app.Settings.StackID, fmt.Sprintf("/apppack-pipeline-%s/", app.Name))
	} else {
		cfg, appRole, err := auth.AppAWSSession(name, sessionDuration)
		if err != nil {
			return nil, err
		}

		app.Pipeline = appRole.Pipeline
		app.Session = cfg
		app.AWS = apppackaws.New(cfg)
	}

	if !app.Pipeline && app.ReviewApp != nil {
		return nil, fmt.Errorf("%s is a standard app and can't have review apps", name)
	}

	return &app, nil
}
