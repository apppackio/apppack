package app

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/dynamodb/dynamodbattribute"
	"github.com/aws/aws-sdk-go/service/ecs"
	"github.com/aws/aws-sdk-go/service/ssm"
	"github.com/lincolnloop/apppack/auth"
	"github.com/logrusorgru/aurora"
)

var maxLifetime = 12 * 60 * 60
var waitForConnect = 60

var shellBackgroundCommand = []string{
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
	Name      string
	Session   *session.Session
	Settings  *Settings
	ECSConfig *ECSConfig
}
type settingsItem struct {
	PrimaryID   string   `json:"primary_id"`
	SecondaryID string   `json:"secondary_id"`
	Settings    Settings `json:"value"`
}

type Settings struct {
	Shell struct {
		Command    string `json:"command"`
		TaskFamily string `json:"task_family"`
	} `json:"shell"`
	DBUtils struct {
		ShellTaskFamily string `json:"shell_task_family"`
	} `json:"dbutils"`
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
	RunTaskArgs ecs.RunTaskInput `locationName:"run_task_args"`
}

func ddbItem(sess *session.Session, primaryID string, secondaryID string) (*map[string]*dynamodb.AttributeValue, error) {
	ddbSvc := dynamodb.New(sess)
	result, err := ddbSvc.GetItem(&dynamodb.GetItemInput{
		TableName: aws.String("paaws"),
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

// StartShellTask start a new shell task on ECS
func (a *App) StartShellTask(taskFamily *string) (*ecs.RunTaskOutput, error) {
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
	runTaskArgs := a.ECSConfig.RunTaskArgs
	command := []*string{}
	for i := range shellBackgroundCommand {
		command = append(command, &shellBackgroundCommand[i])
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
				Command: command,
			},
		},
	}
	return ecsSvc.RunTask(&runTaskArgs)
}

// ConnectToTask open a SSM Session to the Docker host and exec into container
func (a *App) ConnectToTask(task *ecs.Task, cmd *string) error {
	binaryPath, err := exec.LookPath("session-manager-plugin")
	if err != nil {
		fmt.Println(aurora.Red("AWS Session Manager plugin was not found on the path. Install it to use this feature."))
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
	region := "us-east-1" //TODO get from auth info
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
	err = syscall.Exec(binaryPath, []string{"session-manager-plugin", string(arg1), region, "StartSession", "", string(arg2), fmt.Sprintf("https://ssm.%s.amazonaws.com", region)}, os.Environ())
	if err != nil {
		return err
	}
	return nil
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
