package stacks

import (
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/AlecAivazis/survey/v2"
	"github.com/AlecAivazis/survey/v2/core"
	"github.com/apppackio/apppack/ddb"
	"github.com/apppackio/apppack/ui"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudformation"
	"github.com/getsentry/sentry-go"
	"github.com/logrusorgru/aurora"
	"github.com/sirupsen/logrus"
)

var stackHasFailure = false

func CloudformationStackURL(region, stackID *string) string {
	return fmt.Sprintf("https://%s.console.aws.amazon.com/cloudformation/home#/stacks/events?stackId=%s", *region, url.QueryEscape(*stackID))
}

func AskForCluster(sess *session.Session, verbose, helpText string, response interface{}) error {
	clusters, err := ddb.ListClusters(sess)
	if err != nil {
		return err
	}
	if len(clusters) == 0 {
		return fmt.Errorf("no AppPack clusters are setup")
	}
	return ui.AskQuestions([]*ui.QuestionExtra{
		{
			Verbose:  verbose,
			HelpText: helpText,
			Question: &survey.Question{
				Name: "ClusterStackName",
				Prompt: &survey.Select{
					Message: "Cluster",
					Options: clusters,
				},
				Transform: clusterSelectTransform,
			},
		},
	}, response)
}

// waitForCloudformationStack displays the progress of a Stack while it waits for it to complete
func waitForCloudformationStack(cfnSvc *cloudformation.CloudFormation, stackName string) (*cloudformation.Stack, error) {
	ui.StartSpinner()
	stackDesc, err := cfnSvc.DescribeStacks(&cloudformation.DescribeStacksInput{
		StackName: &stackName,
	})
	if err != nil {
		return nil, err
	}
	stack := stackDesc.Stacks[0]

	if strings.HasSuffix(*stack.StackStatus, "_COMPLETE") || strings.HasSuffix(*stack.StackStatus, "_FAILED") {
		ui.Spinner.Stop()
		return stack, nil
	}
	stackresources, err := cfnSvc.DescribeStackResources(&cloudformation.DescribeStackResourcesInput{
		StackName: &stackName,
	})
	if err != nil {
		return nil, err
	}

	var inProgress []string
	var created []string
	var deleted []string
	var failed []string
	// sort oldest to newest so we can catch the first error in the stack
	sort.Slice(stackresources.StackResources, func(i, j int) bool {
		return stackresources.StackResources[i].Timestamp.Before(*stackresources.StackResources[j].Timestamp)
	})

	for _, resource := range stackresources.StackResources {
		// CREATE_IN_PROGRESS | CREATE_FAILED | CREATE_COMPLETE | DELETE_IN_PROGRESS | DELETE_FAILED | DELETE_COMPLETE | DELETE_SKIPPED | UPDATE_IN_PROGRESS | UPDATE_FAILED | UPDATE_COMPLETE | IMPORT_FAILED | IMPORT_COMPLETE | IMPORT_IN_PROGRESS | IMPORT_ROLLBACK_IN_PROGRESS | IMPORT_ROLLBACK_FAILED | IMPORT_ROLLBACK_COMPLETE
		if strings.HasSuffix(*resource.ResourceStatus, "_FAILED") {
			// only warn on the first failure
			// failures will cascade and end up being extra noise
			if !stackHasFailure {
				ui.Spinner.Stop()
				ui.PrintError(fmt.Sprintf("%s failed: %s", *resource.LogicalResourceId, *resource.ResourceStatusReason))
				ui.StartSpinner()
				stackHasFailure = true
			}
			failed = append(failed, *resource.ResourceStatus)
		} else if strings.HasSuffix(*resource.ResourceStatus, "_IN_PROGRESS") {
			inProgress = append(inProgress, *resource.ResourceStatus)
		} else if *resource.ResourceStatus == "CREATE_COMPLETE" {
			created = append(created, *resource.ResourceStatus)
		} else if *resource.ResourceStatus == DeleteComplete {
			deleted = append(deleted, *resource.ResourceStatus)
		}
	}
	status := fmt.Sprintf(" resources: %d in progress / %d created / %d deleted", len(inProgress), len(created), len(deleted))
	if len(failed) > 0 {
		status = fmt.Sprintf("%s / %d failed", status, len(failed))
	}
	ui.Spinner.Suffix = status
	time.Sleep(5 * time.Second)
	return waitForCloudformationStack(cfnSvc, stackName)
}

// retryStackCreation will attempt to destroy and recreate the stack
func retryStackCreation(sess *session.Session, stackID *string, input *cloudformation.CreateStackInput) (*cloudformation.Stack, error) {
	cfnSvc := cloudformation.New(sess)
	ui.PrintWarning("stack creation failed")
	fmt.Println("retrying operation... deleting and recreating stack")
	sentry.CaptureException(fmt.Errorf("Stack creation failed: %s", *input.StackName))

	defer sentry.Flush(time.Second * 5)
	_, err := cfnSvc.DeleteStack(&cloudformation.DeleteStackInput{StackName: stackID})
	if err != nil {
		return nil, err
	}
	stack, err := waitForCloudformationStack(cfnSvc, *stackID)
	if err != nil {
		return nil, err
	}
	if *stack.StackStatus != DeleteComplete {
		err = fmt.Errorf("Stack destruction failed: %s", *stack.StackName)
		sentry.CaptureException(err)
		fmt.Printf("%s", aurora.Bold(aurora.White(
			fmt.Sprintf(
				"Unable to destroy stack. Check Cloudformation console for more details:\n%s",
				CloudformationStackURL(sess.Config.Region, stackID),
			),
		)))
		return nil, err
	}
	fmt.Println("successfully deleted failed stack...")
	return CreateStackAndWait(sess, input)
}

var retry = true

func CreateStackAndWait(sess *session.Session, stackInput *cloudformation.CreateStackInput) (*cloudformation.Stack, error) {
	cfnSvc := cloudformation.New(sess)
	stackOutput, err := cfnSvc.CreateStack(stackInput)
	if err != nil {
		return nil, err
	}
	ui.Spinner.Stop()
	fmt.Println(aurora.Faint(*stackOutput.StackId))
	stack, err := waitForCloudformationStack(cfnSvc, *stackInput.StackName)
	if err != nil {
		return nil, err
	}
	if retry && *stack.StackStatus == "ROLLBACK_COMPLETE" {
		retry = false
		return retryStackCreation(sess, stack.StackId, stackInput)
	}
	return stack, err
}

func UpdateStackAndWait(sess *session.Session, stackInput *cloudformation.UpdateStackInput) (*cloudformation.Stack, error) {
	cfnSvc := cloudformation.New(sess)
	_, err := cfnSvc.UpdateStack(stackInput)
	if err != nil {
		return nil, err
	}
	return waitForCloudformationStack(cfnSvc, *stackInput.StackName)
}

func CreateChangeSetAndWait(sess *session.Session, changesetInput *cloudformation.CreateChangeSetInput) (*cloudformation.DescribeChangeSetOutput, error) {
	cfnSvc := cloudformation.New(sess)
	_, err := cfnSvc.CreateChangeSet(changesetInput)
	if err != nil {
		return nil, err
	}
	describeChangeSetInput := cloudformation.DescribeChangeSetInput{
		ChangeSetName: changesetInput.ChangeSetName,
		StackName:     changesetInput.StackName,
	}
	if err = cfnSvc.WaitUntilChangeSetCreateComplete(&describeChangeSetInput); err != nil {
		return nil, err
	}
	return cfnSvc.DescribeChangeSet(&describeChangeSetInput)
}

// DeleteStackAndWait will execute the PreDelete hook, delete the stack and wait for it to complete,
// then, if successful, execute the PostDelete hook.
func DeleteStackAndWait(sess *session.Session, stack Stack) (*cloudformation.Stack, error) {
	if err := stack.PreDelete(sess); err != nil {
		return nil, err
	}
	cfnSvc := cloudformation.New(sess)
	_, err := cfnSvc.DeleteStack(&cloudformation.DeleteStackInput{
		StackName: stack.GetStack().StackId,
	})
	if err != nil {
		return nil, err
	}
	cfnStack, err := waitForCloudformationStack(cfnSvc, *stack.GetStack().StackId)
	if err == nil && *cfnStack.StackStatus == DeleteComplete {
		if err := stack.PostDelete(sess, nil); err != nil {
			logrus.WithFields(logrus.Fields{"err": err}).Warning("post-delete failed")
			return nil, err
		}
	}
	return cfnStack, err
}

// clusterSelectTransform converts `{name}` -> `{stackName}`
func clusterSelectTransform(ans interface{}) interface{} {
	o, ok := ans.(core.OptionAnswer)
	if !ok {
		return ans
	}
	if o.Value != "" {
		o.Value = fmt.Sprintf(clusterStackNameTmpl, o.Value)
	}
	return o
}
