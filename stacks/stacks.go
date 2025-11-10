package stacks

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/AlecAivazis/survey/v2"
	"github.com/AlecAivazis/survey/v2/core"
	"github.com/apppackio/apppack/ddb"
	"github.com/apppackio/apppack/ui"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"github.com/getsentry/sentry-go"
	"github.com/logrusorgru/aurora"
	"github.com/sirupsen/logrus"
)

var stackHasFailure = false

func CloudformationStackURL(region, stackID *string) string {
	return fmt.Sprintf("https://%s.console.aws.amazon.com/cloudformation/home#/stacks/events?stackId=%s", *region, url.QueryEscape(*stackID))
}

func AskForCluster(cfg aws.Config, verbose, helpText string, response interface{}) error {
	clusters, err := ddb.ListClusters(cfg)
	if err != nil {
		return err
	}

	if len(clusters) == 0 {
		return errors.New("no AppPack clusters are setup")
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
func waitForCloudformationStack(cfnSvc *cloudformation.Client, stackName string) (*types.Stack, error) {
	ui.StartSpinner()

	stackDesc, err := cfnSvc.DescribeStacks(context.Background(), &cloudformation.DescribeStacksInput{
		StackName: &stackName,
	})
	if err != nil {
		return nil, err
	}

	stack := stackDesc.Stacks[0]

	if strings.HasSuffix(string(stack.StackStatus), "_COMPLETE") || strings.HasSuffix(string(stack.StackStatus), "_FAILED") {
		ui.Spinner.Stop()

		return &stack, nil
	}

	stackresources, err := cfnSvc.DescribeStackResources(context.Background(), &cloudformation.DescribeStackResourcesInput{
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
		if strings.HasSuffix(string(resource.ResourceStatus), "_FAILED") {
			// only warn on the first failure
			// failures will cascade and end up being extra noise
			if !stackHasFailure {
				ui.Spinner.Stop()
				ui.PrintError(fmt.Sprintf("%s failed: %s", *resource.LogicalResourceId, *resource.ResourceStatusReason))
				ui.StartSpinner()

				stackHasFailure = true
			}

			failed = append(failed, string(resource.ResourceStatus))
		} else if strings.HasSuffix(string(resource.ResourceStatus), "_IN_PROGRESS") {
			inProgress = append(inProgress, string(resource.ResourceStatus))
		} else if resource.ResourceStatus == types.ResourceStatusCreateComplete {
			created = append(created, string(resource.ResourceStatus))
		} else if resource.ResourceStatus == types.ResourceStatusDeleteComplete {
			deleted = append(deleted, string(resource.ResourceStatus))
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
func retryStackCreation(cfg aws.Config, stackID *string, input *cloudformation.CreateStackInput) (*types.Stack, error) {
	cfnSvc := cloudformation.NewFromConfig(cfg)

	ui.PrintWarning("stack creation failed")
	fmt.Println("retrying operation... deleting and recreating stack")
	sentry.CaptureException(fmt.Errorf("Stack creation failed: %s", *input.StackName))

	defer sentry.Flush(time.Second * 5)

	_, err := cfnSvc.DeleteStack(context.Background(), &cloudformation.DeleteStackInput{StackName: stackID})
	if err != nil {
		return nil, err
	}

	stack, err := waitForCloudformationStack(cfnSvc, *stackID)
	if err != nil {
		return nil, err
	}

	if stack.StackStatus != types.StackStatusDeleteComplete {
		err = fmt.Errorf("Stack destruction failed: %s", *stack.StackName)
		sentry.CaptureException(err)
		fmt.Printf("%s", aurora.Bold(aurora.White(
			"Unable to destroy stack. Check Cloudformation console for more details:\n"+CloudformationStackURL(&cfg.Region, stackID),
		)))

		return nil, err
	}

	fmt.Println("successfully deleted failed stack...")

	return CreateStackAndWait(cfg, input)
}

var retry = true

func CreateStackAndWait(cfg aws.Config, stackInput *cloudformation.CreateStackInput) (*types.Stack, error) {
	cfnSvc := cloudformation.NewFromConfig(cfg)

	stackOutput, err := cfnSvc.CreateStack(context.Background(), stackInput)
	if err != nil {
		return nil, err
	}

	ui.Spinner.Stop()
	fmt.Println(aurora.Faint(*stackOutput.StackId))

	stack, err := waitForCloudformationStack(cfnSvc, *stackInput.StackName)
	if err != nil {
		return nil, err
	}

	if retry && stack.StackStatus == types.StackStatusRollbackComplete {
		retry = false

		return retryStackCreation(cfg, stack.StackId, stackInput)
	}

	return stack, err
}

func UpdateStackAndWait(cfg aws.Config, stackInput *cloudformation.UpdateStackInput) (*types.Stack, error) {
	cfnSvc := cloudformation.NewFromConfig(cfg)

	_, err := cfnSvc.UpdateStack(context.Background(), stackInput)
	if err != nil {
		return nil, err
	}

	return waitForCloudformationStack(cfnSvc, *stackInput.StackName)
}

func CreateChangeSetAndWait(cfg aws.Config, changesetInput *cloudformation.CreateChangeSetInput) (*cloudformation.DescribeChangeSetOutput, error) {
	cfnSvc := cloudformation.NewFromConfig(cfg)

	if _, err := cfnSvc.CreateChangeSet(context.Background(), changesetInput); err != nil {
		return nil, err
	}

	describeChangeSetInput := cloudformation.DescribeChangeSetInput{
		ChangeSetName: changesetInput.ChangeSetName,
		StackName:     changesetInput.StackName,
	}

	changeSet, err := cfnSvc.DescribeChangeSet(context.Background(), &describeChangeSetInput)
	if err != nil {
		return nil, err
	}

	if changeSet.Status == types.ChangeSetStatusFailed &&
		strings.Contains(*changeSet.StatusReason, "didn't contain changes") {
		return nil, fmt.Errorf("no changes detected in stack %s, skipping execution", *changesetInput.StackName)
	}

	waiter := cloudformation.NewChangeSetCreateCompleteWaiter(cfnSvc)
	if err := waiter.Wait(context.Background(), &describeChangeSetInput, 5*time.Minute); err != nil {
		return nil, err
	}

	return changeSet, nil
}

// DeleteStackAndWait will execute the PreDelete hook, delete the stack and wait for it to complete,
// then, if successful, execute the PostDelete hook.
func DeleteStackAndWait(cfg aws.Config, stack Stack) (*types.Stack, error) {
	if err := stack.PreDelete(cfg); err != nil {
		return nil, err
	}

	cfnSvc := cloudformation.NewFromConfig(cfg)

	_, err := cfnSvc.DeleteStack(context.Background(), &cloudformation.DeleteStackInput{
		StackName: stack.GetStack().StackId,
	})
	if err != nil {
		return nil, err
	}

	cfnStack, err := waitForCloudformationStack(cfnSvc, *stack.GetStack().StackId)
	if err == nil && cfnStack.StackStatus == types.StackStatusDeleteComplete {
		if err := stack.PostDelete(cfg, nil); err != nil {
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
