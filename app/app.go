package app

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/apppackio/apppack/auth"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/codebuild"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/dynamodb/dynamodbattribute"
	"github.com/aws/aws-sdk-go/service/ecs"
	"github.com/aws/aws-sdk-go/service/eventbridge"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/ssm"
	"github.com/google/uuid"
	"github.com/logrusorgru/aurora"
	"github.com/sirupsen/logrus"
)

var maxLifetime = 12 * 60 * 60
var waitForConnect = 60

var ShellBackgroundCommand = []string{
	"/bin/sh",
	"-c",
	strings.Join([]string{
		"STOP=$(($(date +%s)+" + fmt.Sprintf("%d", maxLifetime) + "))",
		// Give user time to connect
		"sleep " + fmt.Sprintf("%d", waitForConnect),
		// As long as a user has a shell open, this task will keep running
		"while true",
		"do EXECCMD=\"$(pgrep -f ssm-session-worker\\ ecs-execute-command | wc -l)\"",
		"test \"$EXECCMD\" -eq 0 && exit",
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
	Session               *session.Session
	Settings              *Settings
	ECSConfig             *ECSConfig
	DeployStatus          *DeployStatus
	PendingDeployStatuses []*DeployStatus
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
	PrimaryID   string   `json:"primary_id"`
	SecondaryID string   `json:"secondary_id"`
	Settings    Settings `json:"value"`
}

type Settings struct {
	Cluster struct {
		ARN  string `json:"arn"`
		Name string `json:"name"`
	} `json:"cluster"`
	Domains []string `json:"domains"`
	Shell   struct {
		Command    string `json:"command"`
		TaskFamily string `json:"task_family"`
	} `json:"shell"`
	DBUtils struct {
		ShellTaskFamily    string `json:"shell_task_family"`
		S3Bucket           string `json:"s3_bucket"`
		DumpLoadTaskFamily string `json:"dumpload_task_family"`
		Engine             string `json:"engine"`
	} `json:"dbutils"`
	CodebuildProject struct {
		Name string `json:"name"`
	} `json:"codebuild_project"`
	LogGroup struct {
		Name string `json:"name"`
	} `json:"log_group"`
	StackID string `json:"stack_id"`
}

type deployStatusItem struct {
	PrimaryID    string       `json:"primary_id"`
	SecondaryID  string       `json:"secondary_id"`
	DeployStatus DeployStatus `json:"value"`
}

type DeployStatus struct {
	Phase       string    `json:"phase"`
	Processes   []Process `json:"processes"`
	BuildID     string    `json:"build_id"`
	LastUpdate  int64     `json:"last_update"`
	Commit      string    `json:"commit"`
	BuildNumber int       `json:"build_number"`
	Failed      bool      `json:"failed"`
}

func (d *DeployStatus) FindProcess(name string) (*Process, error) {
	for _, p := range d.Processes {
		if p.Name == name {
			return &p, nil
		}
	}
	return nil, fmt.Errorf("process '%s' not found", name)
}

type Process struct {
	Name         string `json:"name"`
	CPU          string `json:"cpu"`
	Memory       string `json:"memory"`
	MinProcesses int    `json:"min_processes"`
	MaxProcesses int    `json:"max_processes"`
	State        string `json:"state"`
	Command      string `json:"command"`
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

var FargateSupportedConfigurations = []ECSSizeConfiguration{
	{CPU: 256, Memory: 512},
	{CPU: 256, Memory: 1024},
	{CPU: 256, Memory: 2 * 1024},
	{CPU: 512, Memory: 1024},
	{CPU: 512, Memory: 2 * 1024},
	{CPU: 512, Memory: 3 * 1024},
	{CPU: 512, Memory: 4 * 1024},
	{CPU: 1024, Memory: 2 * 1024},
	{CPU: 1024, Memory: 3 * 1024},
	{CPU: 1024, Memory: 4 * 1024},
	{CPU: 1024, Memory: 5 * 1024},
	{CPU: 1024, Memory: 6 * 1024},
	{CPU: 1024, Memory: 7 * 1024},
	{CPU: 1024, Memory: 8 * 1024},
	{CPU: 2 * 1024, Memory: 4 * 1024},
	{CPU: 2 * 1024, Memory: 5 * 1024},
	{CPU: 2 * 1024, Memory: 6 * 1024},
	{CPU: 2 * 1024, Memory: 7 * 1024},
	{CPU: 2 * 1024, Memory: 8 * 1024},
	{CPU: 2 * 1024, Memory: 9 * 1024},
	{CPU: 2 * 1024, Memory: 10 * 1024},
	{CPU: 2 * 1024, Memory: 11 * 1024},
	{CPU: 2 * 1024, Memory: 12 * 1024},
	{CPU: 2 * 1024, Memory: 13 * 1024},
	{CPU: 2 * 1024, Memory: 14 * 1024},
	{CPU: 2 * 1024, Memory: 15 * 1024},
	{CPU: 2 * 1024, Memory: 16 * 1024},
	{CPU: 4 * 1024, Memory: 8 * 1024},
	{CPU: 4 * 1024, Memory: 9 * 1024},
	{CPU: 4 * 1024, Memory: 10 * 1024},
	{CPU: 4 * 1024, Memory: 11 * 1024},
	{CPU: 4 * 1024, Memory: 12 * 1024},
	{CPU: 4 * 1024, Memory: 13 * 1024},
	{CPU: 4 * 1024, Memory: 14 * 1024},
	{CPU: 4 * 1024, Memory: 15 * 1024},
	{CPU: 4 * 1024, Memory: 16 * 1024},
	{CPU: 4 * 1024, Memory: 17 * 1024},
	{CPU: 4 * 1024, Memory: 18 * 1024},
	{CPU: 4 * 1024, Memory: 19 * 1024},
	{CPU: 4 * 1024, Memory: 20 * 1024},
	{CPU: 4 * 1024, Memory: 21 * 1024},
	{CPU: 4 * 1024, Memory: 22 * 1024},
	{CPU: 4 * 1024, Memory: 23 * 1024},
	{CPU: 4 * 1024, Memory: 24 * 1024},
	{CPU: 4 * 1024, Memory: 25 * 1024},
	{CPU: 4 * 1024, Memory: 26 * 1024},
	{CPU: 4 * 1024, Memory: 27 * 1024},
	{CPU: 4 * 1024, Memory: 28 * 1024},
	{CPU: 4 * 1024, Memory: 29 * 1024},
	{CPU: 4 * 1024, Memory: 30 * 1024},
}

func (a *App) IsReviewApp() bool {
	return a.ReviewApp != nil
}

func (a *App) IsFargate() (bool, error) {
	err := a.LoadECSConfig()
	if err != nil {
		return false, err
	}
	return *a.ECSConfig.RunTaskArgs.LaunchType == "FARGATE", nil
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
	return fmt.Errorf("unsupported cpu/memory configuration -- see https://docs.aws.amazon.com/AmazonECS/latest/developerguide/task-cpu-memory-error.html")
}

func (a *App) ReviewAppSettings() (*Settings, error) {
	if !a.IsReviewApp() {
		return nil, fmt.Errorf("only review apps have review app settings")
	}

	Item, err := ddbItem(a.Session, fmt.Sprintf("APP#%s:%s", a.Name, *a.ReviewApp), "settings")
	if err != nil {
		return nil, err
	}
	i := settingsItem{}

	err = dynamodbattribute.UnmarshalMap(*Item, &i)
	if err != nil {
		return nil, err
	}
	return &i.Settings, nil
}

func (a *App) ShellTaskFamily() (*string, error) {
	if a.IsReviewApp() {
		return aws.String(fmt.Sprintf("%s-pr%s-shell", a.Name, *a.ReviewApp)), nil
	}
	err := a.LoadSettings()
	if err != nil {
		return nil, err
	}
	settings := a.Settings

	return &settings.Shell.TaskFamily, nil
}

// URL is used to lookup the app url from settings
// pipelines need to do this for their review apps so it is passed in as an argument
func (a *App) URL(reviewApp *string) (*string, error) {
	var settings *Settings
	var err error
	if reviewApp != nil {
		a.ReviewApp = reviewApp
		settings, err = a.ReviewAppSettings()
		if err != nil {
			return nil, err
		}
		a.ReviewApp = nil
	} else if a.IsReviewApp() {
		settings, err = a.ReviewAppSettings()
		if err != nil {
			return nil, err
		}
	} else {
		err := a.LoadSettings()
		if err != nil {
			return nil, err
		}
		settings = a.Settings
	}

	return aws.String(fmt.Sprintf("https://%s", settings.Domains[0])), nil
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

func (a *App) ddbItem(key string) (*map[string]*dynamodb.AttributeValue, error) {
	if !a.IsReviewApp() {
		return ddbItem(a.Session, fmt.Sprintf("APP#%s", a.Name), key)
	}
	// TODO: move DEPLOYSTATUS to standard review app location
	if strings.HasPrefix(key, "CONFIG") || key == "settings" || strings.HasPrefix(key, "DEPLOYSTATUS") {
		return ddbItem(a.Session, fmt.Sprintf("APP#%s", a.Name), key)
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
	err = dynamodbattribute.NewDecoder(func(d *dynamodbattribute.Decoder) {
		d.TagKey = "locationName"
	}).Decode(&dynamodb.AttributeValue{M: *Item}, &i)
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
		key = strings.Join([]string{key, *a.ReviewApp}, "#")
	}
	if buildARN != "" {
		key = strings.Join([]string{key, buildARN}, "#")
	}
	Item, err := a.ddbItem(key)
	if err != nil {
		return nil, err
	}
	i := deployStatusItem{}

	err = dynamodbattribute.UnmarshalMap(*Item, &i)
	if err != nil {
		return nil, err
	}
	return &i.DeployStatus, nil
}

// LoadDeployStatus will set the app.DeployStatus value from DDB
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

	err = dynamodbattribute.UnmarshalMap(*Item, &i)
	if err != nil {
		return err
	}
	a.Settings = &i.Settings
	return nil
}

// StartTask start a new task on ECS
func (a *App) StartTask(taskFamily *string, command []string, taskOverride *ecs.TaskOverride, fargate bool) (*ecs.Task, error) {
	ecsSvc := ecs.New(a.Session)
	err := a.LoadSettings()
	if err != nil {
		return nil, err
	}
	err = a.LoadECSConfig()
	if err != nil {
		return nil, err
	}
	taskDefn, err := ecsSvc.DescribeTaskDefinition(&ecs.DescribeTaskDefinitionInput{
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

	cmd := []*string{}
	for i := range command {
		cmd = append(cmd, &command[i])
	}
	email, err := auth.WhoAmI()
	if err != nil {
		return nil, err
	}
	startedBy := fmt.Sprintf("apppack-cli/shell/%s", *email)
	runTaskArgs.TaskDefinition = taskDefn.TaskDefinition.TaskDefinitionArn
	runTaskArgs.StartedBy = &startedBy
	taskOverride.ContainerOverrides = []*ecs.ContainerOverride{
		{
			Name:    taskDefn.TaskDefinition.ContainerDefinitions[0].Name,
			Command: cmd,
		},
	}
	runTaskArgs.Overrides = taskOverride
	ecsTaskOutput, err := ecsSvc.RunTask(&runTaskArgs)
	if err != nil {
		return nil, err
	}
	if len(ecsTaskOutput.Failures) > 0 {
		return nil, fmt.Errorf("RunTask failure: %v", ecsTaskOutput.Failures)
	}
	return ecsTaskOutput.Tasks[0], nil
}

// WaitForTaskStopped waits for a task to be running or complete
func (a *App) WaitForTaskStopped(task *ecs.Task) (*int64, error) {
	ecsSvc := ecs.New(a.Session)
	input := ecs.DescribeTasksInput{
		Cluster: task.ClusterArn,
		Tasks:   []*string{task.TaskArn},
	}
	err := ecsSvc.WaitUntilTasksStopped(&input)
	if err != nil {
		return nil, err
	}
	taskDesc, err := ecsSvc.DescribeTasks(&input)
	if err != nil {
		return nil, err
	}
	task = taskDesc.Tasks[0]
	if *task.StopCode != "EssentialContainerExited" {
		return nil, fmt.Errorf("task %s failed %s: %s", *task.TaskArn, *task.StopCode, *task.StoppedReason)
	}
	return task.Containers[0].ExitCode, nil
}

func (a *App) CreateEcsSession(task ecs.Task, shellCmd string) (*ecs.Session, error) {
	ecsSvc := ecs.New(a.Session)
	err := a.LoadSettings()
	if err != nil {
		return nil, err
	}
	execCmdInput := ecs.ExecuteCommandInput{
		Cluster:     task.ClusterArn,
		Command:     &shellCmd,
		Container:   task.Containers[0].Name,
		Interactive: aws.Bool(true),
		Task:        task.TaskArn,
	}
	retries := 20
	// it takes some time for the SSM agent to startup
	// poll for availability
	for retries > 0 {
		time.Sleep(2 * time.Second)
		out, err := ecsSvc.ExecuteCommand(&execCmdInput)
		if err == nil {
			return out.Session, nil
		} else if aerr, ok := err.(awserr.Error); ok {
			if aerr.Code() != ecs.ErrCodeInvalidParameterException {
				return nil, err
			}
		} else {
			return nil, err
		}
		retries--
	}
	return nil, fmt.Errorf("timeout attempting to connect to SSM Agent")
}

// ConnectToEcsSession open a SSM Session to the Docker host and exec into container
func (a *App) ConnectToEcsSession(ecsSession *ecs.Session) error {
	binaryPath, err := exec.LookPath("session-manager-plugin")
	if err != nil {
		fmt.Println(aurora.Red("AWS Session Manager plugin was not found on the path. Install it locally to use this feature."))
		fmt.Println(aurora.White("https://docs.aws.amazon.com/systems-manager/latest/userguide/session-manager-working-with-install-plugin.html"))
		os.Exit(1)
	}
	region := a.Session.Config.Region
	arg1, err := json.Marshal(ecsSession)
	if err != nil {
		return err
	}
	return syscall.Exec(binaryPath, []string{
		"session-manager-plugin",
		string(arg1),
		*region,
		"StartSession",
	}, os.Environ())
}

// StartBuild starts a new CodeBuild run
func (a *App) StartBuild(createReviewApp bool) (*codebuild.Build, error) {
	codebuildSvc := codebuild.New(a.Session)
	err := a.LoadSettings()
	if err != nil {
		return nil, err
	}
	buildInput := codebuild.StartBuildInput{
		ProjectName: &a.Settings.CodebuildProject.Name,
	}
	if a.IsReviewApp() {
		buildInput.SourceVersion = aws.String(fmt.Sprintf("pr/%s", *a.ReviewApp))
		if createReviewApp {
			buildInput.EnvironmentVariablesOverride = []*codebuild.EnvironmentVariable{
				{
					Name:  aws.String("REVIEW_APP_STATUS"),
					Value: aws.String("created"),
					Type:  aws.String("PLAINTEXT"),
				},
			}
		}
	}
	build, err := codebuildSvc.StartBuild(&buildInput)
	return build.Build, err
}

// ListBuilds lists recent CodeBuild runs
func (a *App) RecentBuilds(count int) ([]BuildStatus, error) {
	ddbSvc := dynamodb.New(a.Session)
	primaryID := fmt.Sprintf("APP#%s", a.Name)
	if a.IsReviewApp() {
		primaryID = fmt.Sprintf("%s:%s", primaryID, *a.ReviewApp)
	}
	logrus.WithFields(logrus.Fields{"count": count}).Debug("fetching build list from DDB")
	ddbResp, err := ddbSvc.Query(&dynamodb.QueryInput{
		TableName:              aws.String("apppack"),
		KeyConditionExpression: aws.String("primary_id = :id1  AND begins_with(secondary_id,:id2)"),
		ExpressionAttributeValues: map[string]*dynamodb.AttributeValue{
			":id1": {S: &primaryID},
			":id2": {S: aws.String("BUILD#")},
		},

		Limit:            aws.Int64(int64(count)),
		ScanIndexForward: aws.Bool(false),
	})
	if err != nil {
		return nil, err
	}
	if ddbResp.Items == nil {
		return nil, fmt.Errorf("could not find any builds")
	}
	i := []BuildStatus{}
	err = dynamodbattribute.UnmarshalListOfMaps(ddbResp.Items, &i)
	if err != nil {
		return nil, err
	}
	if len(i) == 0 {
		return nil, fmt.Errorf("could not find any builds")
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
		err = dynamodbattribute.UnmarshalMap(*item, &build)
		if err != nil {
			return nil, err
		}
		if len(build.Build.Arns) == 0 {
			return nil, fmt.Errorf("build has not started yet -- try again in a few seconds")
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
func (a *App) GetConfig() ([]*ssm.Parameter, error) {
	return SsmParameters(a.Session, a.ConfigPrefix())
}

// SetConfig sets a config value for the app
func (a *App) SetConfig(key, value string, overwrite bool) error {
	parameterName := fmt.Sprintf("%s%s", a.ConfigPrefix(), key)
	ssmSvc := ssm.New(a.Session)
	_, err := ssmSvc.PutParameter(&ssm.PutParameterInput{
		Name:      &parameterName,
		Type:      aws.String("SecureString"),
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
func (a *App) DescribeTasks() ([]*ecs.Task, error) {
	err := a.LoadSettings()
	if err != nil {
		return nil, err
	}
	ecsSvc := ecs.New(a.Session)
	taskARNs := []*string{}
	input := ecs.ListTasksInput{
		Cluster: &a.Settings.Cluster.ARN,
	}
	err = ecsSvc.ListTasksPages(&input, func(resp *ecs.ListTasksOutput, lastPage bool) bool {
		for _, taskARN := range resp.TaskArns {
			if taskARN == nil {
				continue
			}
			taskARNs = append(taskARNs, taskARN)
		}
		return !lastPage
	})
	if err != nil {
		return nil, err
	}
	describeTasksOutput, err := ecsSvc.DescribeTasks(&ecs.DescribeTasksInput{
		Tasks:   taskARNs,
		Cluster: &a.Settings.Cluster.ARN,
		Include: []*string{aws.String("TAGS")},
	})
	if err != nil {
		return nil, err
	}
	appTasks := []*ecs.Task{}
	for _, task := range describeTasksOutput.Tasks {
		isApp := false
		isReviewApp := false
		for _, t := range task.Tags {
			if *t.Key == "apppack:appName" && *t.Value == a.Name {
				isApp = true
			}
			if a.IsReviewApp() {
				if *t.Key == "apppack:reviewApp" && *t.Value == fmt.Sprintf("pr/%s", *a.ReviewApp) {
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

func (a *App) GetECSEvents(service string) ([]*ecs.ServiceEvent, error) {
	ecsSvc := ecs.New(a.Session)
	a.LoadSettings()
	logrus.WithFields(logrus.Fields{"service": service}).Debug("fetching service events")
	serviceStatus, err := ecsSvc.DescribeServices(&ecs.DescribeServicesInput{
		Cluster:  &a.Settings.Cluster.ARN,
		Services: aws.StringSlice([]string{fmt.Sprintf("%s-%s", a.Name, service)}),
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
	a.LoadSettings()
	if err != nil {
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
	err := a.LoadSettings()
	if err != nil {
		return nil, err
	}
	if a.IsReviewApp() {
		return aws.String(fmt.Sprintf("%s-pr%s-dbutils", a.Name, *a.ReviewApp)), nil
	}
	return &a.Settings.DBUtils.DumpLoadTaskFamily, nil
}

func (a *App) DBDump() (*ecs.Task, *s3.GetObjectInput, error) {

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
		&ecs.TaskOverride{},
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
		exec = "mysql"
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
	err := a.SetScaleParameter(processType, nil, nil, &cpu, &memory)
	if err != nil {
		return err
	}
	return nil
}

func (a *App) ScaleProcess(processType string, minProcessCount, maxProcessCount int) error {
	err := a.SetScaleParameter(processType, &minProcessCount, &maxProcessCount, nil, nil)
	if err != nil {
		return err
	}
	return nil
}

// SetScaleParameter updates process count and cpu/ram with any non-nil values provided
// if it is not yet set, the defaults from ECSConfig will be used
func (a *App) SetScaleParameter(processType string, minProcessCount, maxProcessCount, cpu, memory *int) error {
	ssmSvc := ssm.New(a.Session)
	parameterName := fmt.Sprintf("/apppack/apps/%s/scaling", a.Name)
	parameterOutput, err := ssmSvc.GetParameter(&ssm.GetParameterInput{
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
		a.LoadECSConfig()
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
	_, err = ssmSvc.PutParameter(&ssm.PutParameterInput{
		Name:      &parameterName,
		Type:      aws.String("String"),
		Value:     aws.String(string(scalingJSON)),
		Overwrite: aws.Bool(true),
	})
	if err != nil {
		return err
	}
	return nil
}

type ScheduledTask struct {
	Schedule string `json:"schedule"`
	Command  string `json:"command"`
}

// ScheduledTasks lists scheduled tasks for the app
func (a *App) ScheduledTasks() ([]*ScheduledTask, error) {
	ssmSvc := ssm.New(a.Session)
	parameterName := fmt.Sprintf("/apppack/apps/%s/scheduled-tasks", a.Name)
	parameterOutput, err := ssmSvc.GetParameter(&ssm.GetParameterInput{
		Name: &parameterName,
	})
	var tasks []*ScheduledTask
	if err != nil {
		tasks = []*ScheduledTask{}
	} else if err = json.Unmarshal([]byte(*parameterOutput.Parameter.Value), &tasks); err != nil {
		return nil, err
	}
	return tasks, nil
}

// CreateScheduledTask adds a scheduled task for the app
func (a *App) CreateScheduledTask(schedule, command string) ([]*ScheduledTask, error) {
	if err := a.ValidateCronString(schedule); err != nil {
		return nil, err
	}
	ssmSvc := ssm.New(a.Session)
	tasks, err := a.ScheduledTasks()
	if err != nil {
		return nil, err
	}
	tasks = append(tasks, &ScheduledTask{
		Schedule: schedule,
		Command:  command,
	})
	tasksBytes, err := json.Marshal(tasks)
	if err != nil {
		return nil, err
	}
	parameterName := fmt.Sprintf("/apppack/apps/%s/scheduled-tasks", a.Name)
	_, err = ssmSvc.PutParameter(&ssm.PutParameterInput{
		Name:      &parameterName,
		Value:     aws.String(string(tasksBytes)),
		Overwrite: aws.Bool(true),
		Type:      aws.String("String"),
	})
	if err != nil {
		return nil, err
	}
	return tasks, nil
}

// DeleteScheduledTask deletes the scheduled task at the given index
func (a *App) DeleteScheduledTask(idx int) (*ScheduledTask, error) {
	ssmSvc := ssm.New(a.Session)
	tasks, err := a.ScheduledTasks()
	if err != nil {
		return nil, err
	}
	if idx > len(tasks) || idx < 0 {
		return nil, fmt.Errorf("invalid index for task to delete")
	}
	taskToDelete := tasks[idx]
	tasks = append(tasks[:idx], tasks[idx+1:]...)
	tasksBytes, err := json.Marshal(tasks)
	if err != nil {
		return nil, err
	}
	parameterName := fmt.Sprintf("/apppack/apps/%s/scheduled-tasks", a.Name)
	_, err = ssmSvc.PutParameter(&ssm.PutParameterInput{
		Name:      &parameterName,
		Value:     aws.String(string(tasksBytes)),
		Overwrite: aws.Bool(true),
		Type:      aws.String("String"),
	})
	if err != nil {
		return nil, err
	}
	return taskToDelete, nil
}

// ValidateCronString validates a cron schedule rule
func (a *App) ValidateCronString(rule string) error {
	eventSvc := eventbridge.New(a.Session)
	ruleName := fmt.Sprintf("apppack-validate-%s", uuid.New().String())
	_, err := eventSvc.PutRule(&eventbridge.PutRuleInput{
		Name:               &ruleName,
		ScheduleExpression: aws.String(fmt.Sprintf("cron(%s)", rule)),
		State:              aws.String("DISABLED"),
	})
	if err == nil {
		eventSvc.DeleteRule(&eventbridge.DeleteRuleInput{
			Name: &ruleName,
		})
	}
	return err
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
	var sess *session.Session
	app := App{
		Name:      name,
		ReviewApp: reviewApp,
	}

	if awsCredentials {
		sess = session.Must(session.NewSession())
		app.Session = sess
		err := app.LoadSettings()
		if err != nil {
			return nil, err
		}
		// this is a horribly hacky way to figure out if the app is a pipeline, but it works
		app.Pipeline = strings.Contains(app.Settings.StackID, fmt.Sprintf("/apppack-pipeline-%s/", app.Name))
	} else {
		sess, appRole, err := auth.AppAWSSession(name, sessionDuration)
		if err != nil {
			return nil, err
		}
		app.Pipeline = appRole.Pipeline
		app.Session = sess
	}
	if !app.Pipeline && app.ReviewApp != nil {
		return nil, fmt.Errorf("%s is a standard app and can't have review apps", name)
	}
	return &app, nil
}
