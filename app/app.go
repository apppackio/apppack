package app

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"strings"
	"syscall"

	"github.com/apppackio/apppack/auth"
	"github.com/aws/aws-sdk-go/aws"
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
)

var maxLifetime = 12 * 60 * 60
var waitForConnect = 60

var ShellBackgroundCommand = []string{
	"/bin/sh",
	"-c",
	strings.Join([]string{
		// Get initial proc count
		"EXPECTED_PROCS=\"$(ls -1 /proc | grep -c [0-9])\"",
		"STOP=$(($(date +%s)+" + fmt.Sprintf("%d", maxLifetime) + "))",
		// Give user time to connect
		"sleep " + fmt.Sprintf("%d", waitForConnect),
		// Loop until procs are less than or equal to initial count
		// As long as a user has a shell open, this task will keep running
		"while true",
		"do PROCS=\"$(ls -1 /proc | grep -c [0-9])\"",
		"test \"$PROCS\" -le \"$EXPECTED_PROCS\" && exit",
		// Timeout if exceeds max lifetime
		"test \"$STOP\" -lt \"$(date +%s)\" && exit 1",
		"sleep 30",
		"done",
	}, "; "),
}

// App is a representation of a AppPack app
type App struct {
	Name                  string
	Session               *session.Session
	Settings              *Settings
	ECSConfig             *ECSConfig
	DeployStatus          *DeployStatus
	PendingDeployStatuses []*DeployStatus
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
}

type deployStatusItem struct {
	PrimaryID    string       `json:"primary_id"`
	SecondaryID  string       `json:"secondary_id"`
	DeployStatus DeployStatus `json:"value"`
}

type DeployStatus struct {
	Phase       string             `json:"phase"`
	Processes   map[string]Process `json:"processes"`
	BuildID     string             `json:"build_id"`
	LastUpdate  int64              `json:"last_update"`
	Commit      string             `json:"commit"`
	BuildNumber int                `json:"build_number"`
	Failed      bool               `json:"failed"`
}

type Process struct {
	CPU          string `json:"cpu"`
	Memory       string `json:"memory"`
	MinProcesses int    `json:"min_processes"`
	MaxProcesses int    `json:"max_processes"`
	State        string `json:"state"`
	Command      string `json:"command"`
}

type appItem struct {
	PrimaryID   string `json:"primary_id"`
	SecondaryID string `json:"secondary_id"`
	App         App    `json:"value"`
}

// locationName is the tag used by aws-sdk internally
// we can use it to load specific AWS Input types from our JSON
type ecsConfigItem struct {
	PrimaryID   string    `locationName:"primary_id"`
	SecondaryID string    `locationName:"secondary_id"`
	ECSConfig   ECSConfig `locationName:"value"`
}

type ECSConfig struct {
	RunTaskArgs        ecs.RunTaskInput `locationName:"run_task_args"`
	RunTaskArgsFargate ecs.RunTaskInput `locationName:"run_task_args_fargate"`
}

func ddbItem(sess *session.Session, primaryID string, secondaryID string) (*map[string]*dynamodb.AttributeValue, error) {
	ddbSvc := dynamodb.New(sess)
	result, err := ddbSvc.GetItem(&dynamodb.GetItemInput{
		TableName: aws.String("apppack"),
		Key: map[string]*dynamodb.AttributeValue{
			"primary_id": {
				S: aws.String(primaryID),
			},
			"secondary_id": {
				S: aws.String(secondaryID),
			},
		},
	})
	if err != nil {
		return nil, err
	}
	if result.Item == nil {
		return nil, fmt.Errorf("Could not find DDB item %s %s", primaryID, secondaryID)
	}
	return &result.Item, nil
}

func (a *App) ddbItem(key string) (*map[string]*dynamodb.AttributeValue, error) {
	return ddbItem(a.Session, fmt.Sprintf("APP#%s", a.Name), key)
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
func (a *App) StartTask(taskFamily *string, command []string, fargate bool) (*ecs.RunTaskOutput, error) {
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
	runTaskArgs.Overrides = &ecs.TaskOverride{
		ContainerOverrides: []*ecs.ContainerOverride{
			{
				Name:    taskDefn.TaskDefinition.ContainerDefinitions[0].Name,
				Command: cmd,
			},
		},
	}
	return ecsSvc.RunTask(&runTaskArgs)
}

// WaitForTaskRunning waits for a task to be running or complete
func (a *App) WaitForTaskRunning(task *ecs.Task) error {
	ecsSvc := ecs.New(a.Session)
	return ecsSvc.WaitUntilTasksRunning(&ecs.DescribeTasksInput{
		Cluster: task.ClusterArn,
		Tasks:   []*string{task.TaskArn},
	})
}

// WaitForTaskStopped waits for a task to be running or complete
func (a *App) WaitForTaskStopped(task *ecs.Task) error {
	ecsSvc := ecs.New(a.Session)
	input := ecs.DescribeTasksInput{
		Cluster: task.ClusterArn,
		Tasks:   []*string{task.TaskArn},
	}
	err := ecsSvc.WaitUntilTasksStopped(&input)
	if err != nil {
		return err
	}
	taskDesc, err := ecsSvc.DescribeTasks(&input)
	if err != nil {
		return err
	}
	task = taskDesc.Tasks[0]
	if *task.StopCode != "EssentialContainerExited" {
		return fmt.Errorf("task %s failed %s: %s", *task.TaskArn, *task.StopCode, *task.StoppedReason)
	}
	if *task.Containers[0].ExitCode > 0 {
		return fmt.Errorf("task %s failed with exit code %d", *task.TaskArn, *task.Containers[0].ExitCode)
	}
	return nil
}

// ConnectToTask open a SSM Session to the Docker host and exec into container
func (a *App) ConnectToTask(task *ecs.Task, cmd *string) error {
	binaryPath, err := exec.LookPath("session-manager-plugin")
	if err != nil {
		fmt.Println(aurora.Red("AWS Session Manager plugin was not found on the path. Install it locally to use this feature."))
		fmt.Println(aurora.White("https://docs.aws.amazon.com/systems-manager/latest/userguide/session-manager-working-with-install-plugin.html"))
		os.Exit(1)
	}
	ecsSvc := ecs.New(a.Session)
	resp, err := ecsSvc.DescribeContainerInstances(&ecs.DescribeContainerInstancesInput{
		Cluster:            task.ClusterArn,
		ContainerInstances: []*string{task.ContainerInstanceArn},
	})
	ssmSvc := ssm.New(a.Session)
	documentName := "AWS-StartInteractiveCommand"
	region := a.Session.Config.Region
	err = a.LoadSettings()
	if err != nil {
		return err
	}
	command := fmt.Sprintf("docker exec -it $(docker ps -q -f label=com.amazonaws.ecs.task-arn=%s) %s", *task.TaskArn, *cmd)
	input := ssm.StartSessionInput{
		DocumentName: &documentName,
		Target:       resp.ContainerInstances[0].Ec2InstanceId,
		Parameters:   map[string][]*string{"command": {&command}},
	}
	startSessionResp, err := ssmSvc.StartSession(&input)
	if err != nil {
		return err
	}
	arg1, err := json.Marshal(startSessionResp)
	if err != nil {
		return err
	}
	arg2, err := json.Marshal(input)
	if err != nil {
		return err
	}
	// session-manager-plugin isn't documented
	// args were determined from here: https://github.com/aws/aws-cli/blob/84f751b71131489afcb5401d8297bb5b3faa29cb/awscli/customizations/sessionmanager.py#L83-L89
	err = syscall.Exec(binaryPath, []string{"session-manager-plugin", string(arg1), *region, "StartSession", "", string(arg2), fmt.Sprintf("https://ssm.%s.amazonaws.com", *region)}, os.Environ())
	if err != nil {
		return err
	}
	return nil
}

// StartBuild starts a new CodeBuild run
func (a *App) StartBuild() (*codebuild.Build, error) {
	codebuildSvc := codebuild.New(a.Session)
	err := a.LoadSettings()
	if err != nil {
		return nil, err
	}
	build, err := codebuildSvc.StartBuild(&codebuild.StartBuildInput{
		ProjectName: &a.Settings.CodebuildProject.Name,
	})
	return build.Build, err
}

// BuildStatus checks the status of a CodeBuild run
func (a *App) BuildStatus(build *codebuild.Build) (*DeployStatus, error) {
	deployStatus, err := a.GetDeployStatus(*build.Arn)
	if err != nil {
		deployStatus, err = a.GetDeployStatus("")
		if err != nil || deployStatus.BuildID != *build.Arn {
			return nil, fmt.Errorf("no status found for build #%d", *build.BuildNumber)
		}
	}
	if deployStatus.Failed {
		return nil, fmt.Errorf("build #%d failed in phase %s", *build.BuildNumber, deployStatus.Phase)
	}
	return deployStatus, nil
}

// ListBuilds lists recent CodeBuild runs
func (a *App) ListBuilds() ([]*codebuild.Build, error) {
	codebuildSvc := codebuild.New(a.Session)
	err := a.LoadSettings()
	if err != nil {
		return nil, err
	}
	buildList, err := codebuildSvc.ListBuildsForProject(&codebuild.ListBuildsForProjectInput{
		ProjectName: &a.Settings.CodebuildProject.Name,
	})
	if err != nil {
		return nil, err
	}
	builds, err := codebuildSvc.BatchGetBuilds(&codebuild.BatchGetBuildsInput{
		Ids: buildList.Ids,
	})
	if err != nil {
		return nil, err
	}
	return builds.Builds, nil
}

// LastBuild retrieves the most recent build
func (a *App) LastBuild() (*codebuild.Build, error) {
	codebuildSvc := codebuild.New(a.Session)
	err := a.LoadSettings()
	if err != nil {
		return nil, err
	}
	buildList, err := codebuildSvc.ListBuildsForProject(&codebuild.ListBuildsForProjectInput{
		ProjectName: &a.Settings.CodebuildProject.Name,
		SortOrder:   aws.String("DESCENDING"),
	})
	if err != nil {
		return nil, err
	}
	if len(buildList.Ids) == 0 {
		return nil, fmt.Errorf("no builds have started for %s", a.Name)
	}
	builds, err := codebuildSvc.BatchGetBuilds(&codebuild.BatchGetBuildsInput{
		Ids: buildList.Ids[0:1],
	})
	if err != nil {
		return nil, err
	}
	return builds.Builds[0], nil
}

// GetBuildArtifact retrieves an artifact stored in S3
func (a *App) GetBuildArtifact(build *codebuild.Build, name string) ([]byte, error) {
	artifactArn := build.Artifacts.Location
	if artifactArn == nil {
		return []byte{}, nil
	}
	s3Path := strings.Join(strings.Split(*artifactArn, ":")[5:], ":")
	pathParts := strings.Split(s3Path, "/")
	s3Svc := s3.New(a.Session)
	obj, err := s3Svc.GetObject(&s3.GetObjectInput{
		Bucket: &pathParts[0],
		Key:    aws.String(strings.Join(append(pathParts[1:], name), "/")),
	})
	if err != nil {
		return []byte{}, err
	}
	return ioutil.ReadAll(obj.Body)
}

// GetConfig returns a list of config parameters for the app
func (a *App) GetConfig() ([]*ssm.Parameter, error) {
	ssmSvc := ssm.New(a.Session)
	var parameters []*ssm.Parameter
	input := ssm.GetParametersByPathInput{
		Path:           aws.String(fmt.Sprintf("/apppack/apps/%s/config/", a.Name)),
		WithDecryption: aws.Bool(true),
	}
	err := ssmSvc.GetParametersByPathPages(&input, func(resp *ssm.GetParametersByPathOutput, lastPage bool) bool {
		for _, parameter := range resp.Parameters {
			if parameter == nil {
				continue
			}
			parameters = append(parameters, parameter)
		}
		return !lastPage
	})
	if err != nil {
		return nil, err
	}
	return parameters, nil
}

// SetConfig sets a config value for the app
func (a *App) SetConfig(key string, value string, overwrite bool) error {
	ssmSvc := ssm.New(a.Session)
	_, err := ssmSvc.PutParameter(&ssm.PutParameterInput{
		Name:      aws.String(fmt.Sprintf("/apppack/apps/%s/config/%s", a.Name, key)),
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
	err = a.LoadDeployStatus()
	if err != nil {
		return nil, err
	}
	ecsSvc := ecs.New(a.Session)
	taskARNs := []*string{}
	for proc := range a.DeployStatus.Processes {
		listTaskOutput, err := ecsSvc.ListTasks(&ecs.ListTasksInput{
			Family:  aws.String(fmt.Sprintf("%s-%s", a.Name, proc)),
			Cluster: &a.Settings.Cluster.ARN,
		})
		if err != nil {
			return nil, err
		}
		taskARNs = append(taskARNs, listTaskOutput.TaskArns...)
	}
	describeTasksOutput, err := ecsSvc.DescribeTasks(&ecs.DescribeTasksInput{
		Tasks:   taskARNs,
		Cluster: &a.Settings.Cluster.ARN,
		Include: []*string{aws.String("TAGS")},
	})
	if err != nil {
		return nil, err
	}
	return describeTasksOutput.Tasks, nil
}

type Scaling struct {
	CPU          int `json:"cpu"`
	Memory       int `json:"memory"`
	MinProcesses int `json:"min_processes"`
	MaxProcesses int `json:"max_processes"`
}

func (a *App) ResizeProcess(processType string, cpu int, memory int) error {
	ssmSvc := ssm.New(a.Session)
	parameterName := fmt.Sprintf("/apppack/apps/%s/scaling", a.Name)
	parameterOutput, err := ssmSvc.GetParameter(&ssm.GetParameterInput{
		Name: &parameterName,
	})
	var scaling map[string]*Scaling
	if err != nil {
		scaling = map[string]*Scaling{}
	} else {
		if err = json.Unmarshal([]byte(*parameterOutput.Parameter.Value), &scaling); err != nil {
			return err
		}
	}
	_, ok := scaling[processType]
	if !ok {
		scaling[processType] = &Scaling{
			CPU:          1024,
			Memory:       2048,
			MinProcesses: 1,
			MaxProcesses: 1,
		}
	}
	scaling[processType].CPU = cpu
	scaling[processType].Memory = memory
	scalingJSON, err := json.Marshal(scaling)
	if err != nil {
		return err
	}
	_, err = ssmSvc.PutParameter(&ssm.PutParameterInput{
		Name:      &parameterName,
		Type:      aws.String("String"),
		Value:     aws.String(fmt.Sprintf("%s", scalingJSON)),
		Overwrite: aws.Bool(true),
	})
	if err != nil {
		return err
	}
	return nil
}

func (a *App) ScaleProcess(processType string, minProcessCount int, maxProcessCount int) error {
	ssmSvc := ssm.New(a.Session)
	parameterName := fmt.Sprintf("/apppack/apps/%s/scaling", a.Name)
	parameterOutput, err := ssmSvc.GetParameter(&ssm.GetParameterInput{
		Name: &parameterName,
	})
	var scaling map[string]*Scaling
	if err != nil {
		scaling = map[string]*Scaling{}
	} else {
		if err = json.Unmarshal([]byte(*parameterOutput.Parameter.Value), &scaling); err != nil {
			return err
		}
	}
	_, ok := scaling[processType]
	if !ok {
		scaling[processType] = &Scaling{
			CPU:          1024,
			Memory:       2048,
			MinProcesses: 1,
			MaxProcesses: 1,
		}
	}
	scaling[processType].MinProcesses = minProcessCount
	scaling[processType].MaxProcesses = maxProcessCount
	scalingJSON, err := json.Marshal(scaling)
	if err != nil {
		return err
	}
	_, err = ssmSvc.PutParameter(&ssm.PutParameterInput{
		Name:      &parameterName,
		Type:      aws.String("String"),
		Value:     aws.String(fmt.Sprintf("%s", scalingJSON)),
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
	} else {
		if err = json.Unmarshal([]byte(*parameterOutput.Parameter.Value), &tasks); err != nil {
			return nil, err
		}
	}
	return tasks, nil
}

// CreateScheduledTask adds a scheduled task for the app
func (a *App) CreateScheduledTask(schedule string, command string) ([]*ScheduledTask, error) {
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
		Value:     aws.String(fmt.Sprintf("%s", tasksBytes)),
		Overwrite: aws.Bool(true),
		Type:      aws.String("String"),
	})
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
		Value:     aws.String(fmt.Sprintf("%s", tasksBytes)),
		Overwrite: aws.Bool(true),
		Type:      aws.String("String"),
	})
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
func Init(name string) (*App, error) {
	sess, err := auth.AwsSession(name)
	if err != nil {
		return nil, err
	}
	app := App{
		Name:    name,
		Session: sess,
	}
	return &app, nil
}
