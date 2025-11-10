package bridge

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"github.com/aws/smithy-go"
)

// stackExists checks if a named Cfn Stack already exists in the region
func StackExists(cfg aws.Config, stackName string) (*bool, error) {
	stack, err := GetStack(cfg, stackName)

	var exists bool

	if err != nil {
		var apiErr smithy.APIError
		if errors.As(err, &apiErr) {
			if apiErr.ErrorCode() == "ValidationError" {
				exists = false

				return &exists, nil
			}
		}

		return nil, err
	}

	exists = stack.StackStatus != types.StackStatusDeleteComplete

	return &exists, nil
}

func GetStackParameter(parameters []types.Parameter, name string) (*string, error) {
	for _, parameter := range parameters {
		if *parameter.ParameterKey == name {
			return parameter.ParameterValue, nil
		}
	}

	return nil, fmt.Errorf("no parameter named %s", name)
}

func GetStackOutput(outputs []types.Output, name string) (*string, error) {
	for _, output := range outputs {
		if *output.OutputKey == name {
			return output.OutputValue, nil
		}
	}

	return nil, fmt.Errorf("no output named %s", name)
}

func GetStack(cfg aws.Config, name string) (*types.Stack, error) {
	cfnSvc := cloudformation.NewFromConfig(cfg)

	stacks, err := cfnSvc.DescribeStacks(context.Background(), &cloudformation.DescribeStacksInput{
		StackName: &name,
	})
	if err != nil {
		return nil, err
	}

	return &stacks.Stacks[0], nil
}

func ApppackStacks(cfg aws.Config) ([]types.Stack, error) {
	cfnSvc := cloudformation.NewFromConfig(cfg)

	var stacks []types.Stack

	paginator := cloudformation.NewDescribeStacksPaginator(cfnSvc, &cloudformation.DescribeStacksInput{})

	for paginator.HasMorePages() {
		resp, err := paginator.NextPage(context.Background())
		if err != nil {
			return nil, err
		}

		for _, stack := range resp.Stacks {
			if strings.HasPrefix(*stack.StackName, "apppack-") {
				stacks = append(stacks, stack)
			}
		}
	}

	return stacks, nil
}
