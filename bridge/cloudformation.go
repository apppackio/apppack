package bridge

import (
	"fmt"

	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudformation"
)

// stackExists checks if a named Cfn Stack already exists in the region
func StackExists(sess *session.Session, stackName string) (*bool, error) {
	stack, err := GetStack(sess, stackName)
	var exists bool
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
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
