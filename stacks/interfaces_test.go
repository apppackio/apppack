package stacks_test

import (
	"testing"

	"github.com/apppackio/apppack/bridge"
	"github.com/apppackio/apppack/stacks"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
)

func TestStructToCloudformationParameters(t *testing.T) {
	t.Parallel()

	p := stacks.AppStackParameters{
		Name:                     "test",
		SQSQueueEnabled:          true,
		LoadBalancerRulePriority: 20,
		AllowedUsers:             []string{"test1", "test2"},
	}

	cfnParams, err := stacks.StructToCloudformationParameters(&p)
	if err != nil {
		t.Errorf("Error converting struct to Cloudformation parameters: %s", err)
	}

	name, err := bridge.GetStackParameter(cfnParams, "Name")
	if err != nil {
		t.Errorf("Error converting struct to Cloudformation parameters: %s", err)
	}

	if *name != p.Name {
		t.Errorf("Name parameter did not match: %s != %s", *name, p.Name)
	}

	sqsQueueEnabled, err := bridge.GetStackParameter(cfnParams, "SQSQueueEnabled")
	if err != nil {
		t.Errorf("Error converting struct to Cloudformation parameters: %s", err)
	}

	if *sqsQueueEnabled != stacks.Enabled {
		t.Errorf("Name parameter did not match: %s != %s", *sqsQueueEnabled, stacks.Enabled)
	}

	loadBalancerRulePriority, err := bridge.GetStackParameter(cfnParams, "LoadBalancerRulePriority")
	if err != nil {
		t.Errorf("Error converting struct to Cloudformation parameters: %s", err)
	}

	if *loadBalancerRulePriority != "20" {
		t.Errorf("Name parameter did not match: %s != %s", *loadBalancerRulePriority, "20")
	}

	allowedUsers, err := bridge.GetStackParameter(cfnParams, "AllowedUsers")
	if err != nil {
		t.Errorf("Error converting struct to Cloudformation parameters: %s", err)
	}

	if *allowedUsers != "test1,test2" {
		t.Errorf("Name parameter did not match: %s != %s", *allowedUsers, "test1,test2")
	}
}

func TestCloudformationParametersToStruct(t *testing.T) {
	t.Parallel()

	cfnParams := []types.Parameter{
		{ParameterKey: aws.String("Name"), ParameterValue: aws.String("test")},
		{ParameterKey: aws.String("SQSQueueEnabled"), ParameterValue: aws.String(stacks.Enabled)},
		{ParameterKey: aws.String("LoadBalancerRulePriority"), ParameterValue: aws.String("20")},
		{ParameterKey: aws.String("AllowedUsers"), ParameterValue: aws.String("test1,test2")},
	}
	p := stacks.AppStackParameters{}

	err := stacks.CloudformationParametersToStruct(&p, cfnParams)
	if err != nil {
		t.Errorf("Error converting Cloudformation parameters to struct: %s", err)
	}

	if p.Name != "test" {
		t.Errorf("Name parameter did not match: %s != %s", p.Name, "test")
	}

	if !p.SQSQueueEnabled {
		t.Errorf("SQSQueueEnabled parameter did not match: %t != %t", p.SQSQueueEnabled, true)
	}

	if p.LoadBalancerRulePriority != 20 {
		t.Errorf("LoadBalancerRulePriority parameter did not match: %d != %d", p.LoadBalancerRulePriority, 20)
	}

	if len(p.AllowedUsers) != 2 {
		t.Errorf("AllowedUsers parameter did not match: %d != %d", len(p.AllowedUsers), 2)
	}

	if p.AllowedUsers[0] != "test1" {
		t.Errorf("AllowedUsers parameter did not match: %s != %s", p.AllowedUsers[0], "test1")
	}

	if p.AllowedUsers[1] != "test2" {
		t.Errorf("AllowedUsers parameter did not match: %s != %s", p.AllowedUsers[1], "test2")
	}
}

func TestCloudformationParametersToStructWithNameMapping(t *testing.T) {
	t.Parallel()

	// Test that CloudFormation parameter "RepositoryUrl" maps to struct field "RepositoryURL" via cfnparam tag
	cfnParams := []*cloudformation.Parameter{
		{ParameterKey: aws.String("Name"), ParameterValue: aws.String("test-app")},
		{ParameterKey: aws.String("RepositoryUrl"), ParameterValue: aws.String("https://github.com/org/repo.git")},
		{ParameterKey: aws.String("Branch"), ParameterValue: aws.String("main")},
	}
	p := stacks.AppStackParameters{}

	// Test reading from CloudFormation
	err := stacks.CloudformationParametersToStruct(&p, cfnParams)
	if err != nil {
		t.Errorf("Error converting Cloudformation parameters to struct: %s", err)
	}

	if p.Name != "test-app" {
		t.Errorf("Name parameter did not match: %s != %s", p.Name, "test-app")
	}

	if p.RepositoryURL != "https://github.com/org/repo.git" {
		t.Errorf("RepositoryURL parameter did not match: %s != %s", p.RepositoryURL, "https://github.com/org/repo.git")
	}

	if p.Branch != "main" {
		t.Errorf("Branch parameter did not match: %s != %s", p.Branch, "main")
	}

	// Test writing to CloudFormation - verify cfnparam tag is used
	p2 := stacks.AppStackParameters{
		Name:          "test-app",
		RepositoryURL: "https://github.com/org/repo.git",
		Branch:        "main",
	}

	params, err := stacks.StructToCloudformationParameters(&p2)
	if err != nil {
		t.Errorf("Error converting struct to Cloudformation parameters: %s", err)
	}

	// Find the RepositoryUrl parameter
	var repoParam *cloudformation.Parameter
	for _, param := range params {
		if *param.ParameterKey == "RepositoryUrl" {
			repoParam = param
			break
		}
	}

	if repoParam == nil {
		t.Errorf("RepositoryUrl parameter not found - cfnparam tag not being used")
	} else if *repoParam.ParameterValue != "https://github.com/org/repo.git" {
		t.Errorf("RepositoryUrl parameter value did not match: %s != %s", *repoParam.ParameterValue, "https://github.com/org/repo.git")
	}
}

func TestPruneUnsupportedParameters(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                string
		supportedParameters []*cloudformation.Parameter
		desiredParameters   []*cloudformation.Parameter
		expectedParameters  []*cloudformation.Parameter
	}{
		{
			name: "preserve unmodified parameters with UsePreviousValue",
			supportedParameters: []*cloudformation.Parameter{
				{ParameterKey: aws.String("Name"), ParameterValue: aws.String("test-app")},
				{ParameterKey: aws.String("Branch"), ParameterValue: aws.String("main")},
				{ParameterKey: aws.String("RepositoryURL"), ParameterValue: aws.String("https://github.com/org/repo.git")},
			},
			desiredParameters: []*cloudformation.Parameter{
				{ParameterKey: aws.String("Branch"), ParameterValue: aws.String("develop")},
			},
			expectedParameters: []*cloudformation.Parameter{
				{ParameterKey: aws.String("Branch"), ParameterValue: aws.String("develop")},
				{ParameterKey: aws.String("Name"), UsePreviousValue: aws.Bool(true)},
				{ParameterKey: aws.String("RepositoryURL"), UsePreviousValue: aws.Bool(true)},
			},
		},
		{
			name: "exclude unsupported parameters",
			supportedParameters: []*cloudformation.Parameter{
				{ParameterKey: aws.String("Name"), ParameterValue: aws.String("test-app")},
				{ParameterKey: aws.String("Branch"), ParameterValue: aws.String("main")},
			},
			desiredParameters: []*cloudformation.Parameter{
				{ParameterKey: aws.String("Branch"), ParameterValue: aws.String("develop")},
				{ParameterKey: aws.String("NewParameter"), ParameterValue: aws.String("value")},
			},
			expectedParameters: []*cloudformation.Parameter{
				{ParameterKey: aws.String("Branch"), ParameterValue: aws.String("develop")},
				{ParameterKey: aws.String("Name"), UsePreviousValue: aws.Bool(true)},
			},
		},
		{
			name: "update multiple parameters and preserve others",
			supportedParameters: []*cloudformation.Parameter{
				{ParameterKey: aws.String("Name"), ParameterValue: aws.String("test-app")},
				{ParameterKey: aws.String("Branch"), ParameterValue: aws.String("main")},
				{ParameterKey: aws.String("RepositoryURL"), ParameterValue: aws.String("https://github.com/org/repo.git")},
				{ParameterKey: aws.String("HealthCheckPath"), ParameterValue: aws.String("/health")},
			},
			desiredParameters: []*cloudformation.Parameter{
				{ParameterKey: aws.String("Branch"), ParameterValue: aws.String("develop")},
				{ParameterKey: aws.String("HealthCheckPath"), ParameterValue: aws.String("/alive")},
			},
			expectedParameters: []*cloudformation.Parameter{
				{ParameterKey: aws.String("Branch"), ParameterValue: aws.String("develop")},
				{ParameterKey: aws.String("HealthCheckPath"), ParameterValue: aws.String("/alive")},
				{ParameterKey: aws.String("Name"), UsePreviousValue: aws.Bool(true)},
				{ParameterKey: aws.String("RepositoryURL"), UsePreviousValue: aws.Bool(true)},
			},
		},
		{
			name:                "empty supported parameters",
			supportedParameters: []*cloudformation.Parameter{},
			desiredParameters: []*cloudformation.Parameter{
				{ParameterKey: aws.String("Name"), ParameterValue: aws.String("test-app")},
			},
			expectedParameters: []*cloudformation.Parameter{},
		},
		{
			name: "all parameters being updated",
			supportedParameters: []*cloudformation.Parameter{
				{ParameterKey: aws.String("Name"), ParameterValue: aws.String("test-app")},
				{ParameterKey: aws.String("Branch"), ParameterValue: aws.String("main")},
			},
			desiredParameters: []*cloudformation.Parameter{
				{ParameterKey: aws.String("Name"), ParameterValue: aws.String("new-app")},
				{ParameterKey: aws.String("Branch"), ParameterValue: aws.String("develop")},
			},
			expectedParameters: []*cloudformation.Parameter{
				{ParameterKey: aws.String("Name"), ParameterValue: aws.String("new-app")},
				{ParameterKey: aws.String("Branch"), ParameterValue: aws.String("develop")},
			},
		},
		{
			name: "preserve CloudFormation parameters unknown to CLI",
			supportedParameters: []*cloudformation.Parameter{
				{ParameterKey: aws.String("Name"), ParameterValue: aws.String("test-app")},
				{ParameterKey: aws.String("Branch"), ParameterValue: aws.String("main")},
				{ParameterKey: aws.String("NewTemplateParameter"), ParameterValue: aws.String("some-value")},
				{ParameterKey: aws.String("AnotherNewParameter"), ParameterValue: aws.String("another-value")},
			},
			desiredParameters: []*cloudformation.Parameter{
				{ParameterKey: aws.String("Branch"), ParameterValue: aws.String("develop")},
			},
			expectedParameters: []*cloudformation.Parameter{
				{ParameterKey: aws.String("Branch"), ParameterValue: aws.String("develop")},
				{ParameterKey: aws.String("Name"), UsePreviousValue: aws.Bool(true)},
				{ParameterKey: aws.String("NewTemplateParameter"), UsePreviousValue: aws.Bool(true)},
				{ParameterKey: aws.String("AnotherNewParameter"), UsePreviousValue: aws.Bool(true)},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := stacks.PruneUnsupportedParameters(tt.supportedParameters, tt.desiredParameters)

			// Check we got the expected number of parameters
			if len(result) != len(tt.expectedParameters) {
				t.Errorf("expected %d parameters, got %d", len(tt.expectedParameters), len(result))
			}

			// Build a map of expected parameters for easier lookup
			expectedMap := make(map[string]*cloudformation.Parameter)
			for _, param := range tt.expectedParameters {
				expectedMap[*param.ParameterKey] = param
			}

			// Check each parameter
			for _, param := range result {
				key := *param.ParameterKey
				expected, ok := expectedMap[key]
				if !ok {
					t.Errorf("unexpected parameter: %s", key)
					continue
				}

				// Check ParameterValue
				if expected.ParameterValue != nil {
					if param.ParameterValue == nil {
						t.Errorf("parameter %s: expected value %s, got nil", key, *expected.ParameterValue)
					} else if *param.ParameterValue != *expected.ParameterValue {
						t.Errorf("parameter %s: expected value %s, got %s", key, *expected.ParameterValue, *param.ParameterValue)
					}
				} else if param.ParameterValue != nil {
					t.Errorf("parameter %s: expected nil value, got %s", key, *param.ParameterValue)
				}

				// Check UsePreviousValue
				if expected.UsePreviousValue != nil {
					if param.UsePreviousValue == nil {
						t.Errorf("parameter %s: expected UsePreviousValue %t, got nil", key, *expected.UsePreviousValue)
					} else if *param.UsePreviousValue != *expected.UsePreviousValue {
						t.Errorf("parameter %s: expected UsePreviousValue %t, got %t", key, *expected.UsePreviousValue, *param.UsePreviousValue)
					}
				} else if param.UsePreviousValue != nil {
					t.Errorf("parameter %s: expected nil UsePreviousValue, got %t", key, *param.UsePreviousValue)
				}
			}
		})
	}
}
