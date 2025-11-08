package bridge

import (
	"errors"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudformation"
)

// stackExists checks if a named Cfn Stack already exists in the region
func StackExists(sess *session.Session, stackName string) (*bool, error) {
	stack, err := GetStack(sess, stackName)

	var exists bool

	if err != nil {
		var aerr awserr.Error
		if errors.As(err, &aerr) {
			if aerr.Code() == "ValidationError" {
				exists = false

				return &exists, nil
			}
		}

		return nil, err
	}

	exists = *stack.StackStatus != cloudformation.StackStatusDeleteComplete

	return &exists, nil
}

func GetStackParameter(parameters []*cloudformation.Parameter, name string) (*string, error) {
	for _, parameter := range parameters {
		if *parameter.ParameterKey == name {
			return parameter.ParameterValue, nil
		}
	}

	return nil, fmt.Errorf("no parameter named %s", name)
}

func GetStackOutput(outputs []*cloudformation.Output, name string) (*string, error) {
	for _, output := range outputs {
		if *output.OutputKey == name {
			return output.OutputValue, nil
		}
	}

	return nil, fmt.Errorf("no output named %s", name)
}

func GetStack(sess *session.Session, name string) (*cloudformation.Stack, error) {
	cfnSvc := cloudformation.New(sess)

	stacks, err := cfnSvc.DescribeStacks(&cloudformation.DescribeStacksInput{
		StackName: &name,
	})
	if err != nil {
		return nil, err
	}

	return stacks.Stacks[0], nil
}

func ApppackStacks(sess *session.Session) ([]*cloudformation.Stack, error) {
	cfnSvc := cloudformation.New(sess)

	var stacks []*cloudformation.Stack

	var token *string

	for {
		resp, err := cfnSvc.DescribeStacks(&cloudformation.DescribeStacksInput{
			NextToken: token,
		})
		if err != nil {
			return nil, err
		}

		for _, stack := range resp.Stacks {
			if strings.HasPrefix(*stack.StackName, "apppack-") {
				stacks = append(stacks, stack)
			}
		}

		if resp.NextToken == nil {
			break
		}

		token = resp.NextToken
	}

	return stacks, nil
}
