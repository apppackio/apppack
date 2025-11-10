package stacks

import (
	"fmt"
	"math/rand"
	"strings"

	"github.com/apppackio/apppack/bridge"
	"github.com/apppackio/apppack/ui"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudformation"
	"github.com/spf13/pflag"
)

// IsReviewAppName checks if a name follows the review app naming convention (pipeline:pr)
func IsReviewAppName(name string) bool {
	return strings.Contains(name, ":")
}

func splitReviewAppName(name *string) (string, string) {
	parts := strings.Split(*name, ":")

	return parts[0], parts[1]
}

type ReviewAppStackParameters struct {
	Name                     string
	PipelineStackName        string
	LoadBalancerRulePriority int

	PrivateS3BucketEnabled bool
	PublicS3BucketEnabled  bool
	SesDomain              bool
	DatabaseAddon          bool
	RedisAddon             bool
	SQSQueueEnabled        bool
	CustomTaskPolicy       bool
}

func (p *ReviewAppStackParameters) Import(parameters []*cloudformation.Parameter) error {
	return CloudformationParametersToStruct(p, parameters)
}

func (p *ReviewAppStackParameters) ToCloudFormationParameters() ([]*cloudformation.Parameter, error) {
	return StructToCloudformationParameters(p)
}

// SetParametersFromPipeline sets parameters for the review app based on output from the pipeline stack
func (p *ReviewAppStackParameters) SetParametersFromPipeline(stack *cloudformation.Stack) error {
	databaseLambda, err := bridge.GetStackOutput(stack.Outputs, "DatabaseManagerLambdaArn")
	if err != nil {
		return err
	}

	p.DatabaseAddon = *databaseLambda != "~"

	redisUserGroupAssociationLambdaArn, err := bridge.GetStackOutput(stack.Outputs, "RedisUserGroupAssociationLambdaArn")
	if err != nil {
		return err
	}

	p.RedisAddon = *redisUserGroupAssociationLambdaArn != "~"

	privateS3Bucket, err := bridge.GetStackOutput(stack.Outputs, "PrivateS3Bucket")
	if err != nil {
		return err
	}

	p.PrivateS3BucketEnabled = *privateS3Bucket != "~"

	publicS3Bucket, err := bridge.GetStackOutput(stack.Outputs, "PublicS3Bucket")
	if err != nil {
		return err
	}

	p.PublicS3BucketEnabled = *publicS3Bucket != "~"

	ses, err := bridge.GetStackOutput(stack.Outputs, "SesDomain")
	if err != nil {
		return err
	}

	p.SesDomain = *ses != "~"

	sqs, err := bridge.GetStackOutput(stack.Outputs, "SQSQueueEnabled")
	if err != nil {
		return err
	}

	p.SQSQueueEnabled = *sqs == Enabled

	customTaskPolicyArn, err := bridge.GetStackOutput(stack.Outputs, "CustomTaskPolicyArn")
	if err != nil {
		return err
	}

	p.CustomTaskPolicy = *customTaskPolicyArn != "~"

	return nil
}

// SetInternalFields updates fields that aren't exposed to the user
func (p *ReviewAppStackParameters) SetInternalFields(sess *session.Session, name *string) error {
	ui.StartSpinner()

	pipeline, pr := splitReviewAppName(name)

	pipelineStack, err := bridge.GetStack(sess, fmt.Sprintf(PipelineStackNameTmpl, pipeline))
	if err != nil {
		return err
	}

	p.PipelineStackName = *pipelineStack.StackName
	if err := p.SetParametersFromPipeline(pipelineStack); err != nil {
		return err
	}

	p.LoadBalancerRulePriority = rand.Intn(50000-200) + 200 // #nosec G404 -- Non-crypto random for LB priority assignment
	p.Name = pr

	ui.Spinner.Stop()

	return nil
}

type ReviewAppStack struct {
	Stack      *cloudformation.Stack
	Parameters *ReviewAppStackParameters
}

func (a *ReviewAppStack) GetParameters() Parameters {
	return a.Parameters
}

func (a *ReviewAppStack) GetStack() *cloudformation.Stack {
	return a.Stack
}

func (a *ReviewAppStack) SetStack(stack *cloudformation.Stack) {
	a.Stack = stack
}

func (*ReviewAppStack) PostCreate(_ *session.Session) error {
	return nil
}

func (*ReviewAppStack) PreDelete(_ *session.Session) error {
	return nil
}

func (*ReviewAppStack) PostDelete(_ *session.Session, _ *string) error {
	return nil
}

func (a *ReviewAppStack) UpdateFromFlags(flags *pflag.FlagSet) error {
	return ui.FlagsToStruct(a.Parameters, flags)
}

func (*ReviewAppStack) AskQuestions(_ *session.Session) error {
	return nil
}

func (*ReviewAppStack) StackName(name *string) *string {
	stackName := fmt.Sprintf(reviewAppStackNameTmpl, strings.ReplaceAll(*name, ":", ""))

	return &stackName
}

func (*ReviewAppStack) StackType() string {
	return "review app"
}

func (*ReviewAppStack) Tags(name *string) []*cloudformation.Tag {
	pipeline, pr := splitReviewAppName(name)

	return []*cloudformation.Tag{
		{Key: aws.String("apppack:appName"), Value: &pipeline},
		{Key: aws.String("apppack:reviewApp"), Value: aws.String("pr" + pr)},
		// {Key: aws.String("apppack:cluster"), Value: aws.String("...")},
		{Key: aws.String("apppack"), Value: aws.String("true")},
	}
}

func (*ReviewAppStack) Capabilities() []*string {
	return []*string{
		aws.String("CAPABILITY_IAM"),
	}
}

func (a *ReviewAppStack) CfnRole(sess *session.Session) (*string, error) {
	stack, err := bridge.GetStack(sess, a.Parameters.PipelineStackName)
	if err != nil {
		return nil, err
	}

	return bridge.GetStackOutput(stack.Outputs, "ReviewAppCfnRoleArn")
}

func (*ReviewAppStack) TemplateURL(release *string) *string {
	url := reviewAppFormationURL
	if release != nil && *release != "" {
		url = strings.Replace(url, "/latest/", fmt.Sprintf("/%s/", *release), 1)
	}

	return &url
}
