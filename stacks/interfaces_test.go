package stacks_test

import (
	"testing"

	"github.com/apppackio/apppack/bridge"
	"github.com/apppackio/apppack/stacks"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/cloudformation"
)

func TestStructToCloudformationParameters(t *testing.T) {
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
	cfnParams := []*cloudformation.Parameter{
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
