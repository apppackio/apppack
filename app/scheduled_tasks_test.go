package app

import (
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ssm"
	"github.com/stretchr/testify/mock"
)

type MockAWS struct {
	mock.Mock
}

func (m *MockAWS) GetParameter(input *ssm.GetParameterInput) (*string, error) {
	args := m.Called(input)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*string), args.Error(1)
}

func (m *MockAWS) PutParameter(input *ssm.PutParameterInput) error {
	args := m.Called(input)
	return args.Error(0)
}

func (m *MockAWS) ValidateEventbridgeCron(rule string) error {
	args := m.Called(rule)
	return args.Error(0)
}

func TestScheduledTasksNoParameter(t *testing.T) {
	a := &App{
		Name: "test",
		aws:  &MockAWS{},
	}
	a.aws.(*MockAWS).On(
		"GetParameter",
		&ssm.GetParameterInput{Name: aws.String("/apppack/apps/test/scheduled-tasks")},
	).Return(nil, fmt.Errorf("parameter not found"))
	tasks, err := a.ScheduledTasks()
	if err != nil {
		t.Error(err)
	}
	if len(tasks) != 0 {
		t.Errorf("expected 0 tasks, got %d", len(tasks))
	}
}

func TestScheduledTasksCreate(t *testing.T) {
	a := &App{
		Name: "test",
		aws:  &MockAWS{},
	}
	parameterName := "/apppack/apps/test/scheduled-tasks"
	a.aws.(*MockAWS).On(
		"GetParameter",
		&ssm.GetParameterInput{Name: &parameterName},
	).Return(nil, fmt.Errorf("parameter not found"))
	schedule := "0/10 * * * ? *"
	a.aws.(*MockAWS).On(
		"ValidateEventbridgeCron",
		schedule,
	).Return(nil)
	command := "echo hello"
	a.aws.(*MockAWS).On(
		"PutParameter",
		&ssm.PutParameterInput{
			Name:      &parameterName,
			Value:     aws.String(fmt.Sprintf("[{\"schedule\":\"%s\",\"command\":\"%s\"}]", schedule, command)),
			Type:      aws.String("String"),
			Overwrite: aws.Bool(true),
		},
	).Return(nil)
	tasks, err := a.CreateScheduledTask(schedule, command)
	if err != nil {
		t.Error(err)
	}
	if len(tasks) != 1 {
		t.Errorf("expected 1 task, got %d", len(tasks))
	}
	if tasks[0].Command != command {
		t.Errorf("expected command %s, got %s", command, tasks[0].Command)
	}
}

func TestScheduledTasksDelete(t *testing.T) {
	a := &App{
		Name: "test",
		aws:  &MockAWS{},
	}
	parameterName := "/apppack/apps/test/scheduled-tasks"
	schedule := "0/10 * * * ? *"
	command := "echo hello"
	a.aws.(*MockAWS).On(
		"GetParameter",
		&ssm.GetParameterInput{Name: &parameterName},
	).Return(aws.String(fmt.Sprintf("[{\"schedule\":\"%s\",\"command\":\"%s\"}]", schedule, command)), nil)
	a.aws.(*MockAWS).On(
		"PutParameter",
		&ssm.PutParameterInput{
			Name:      &parameterName,
			Value:     aws.String("[]"),
			Type:      aws.String("String"),
			Overwrite: aws.Bool(true),
		},
	).Return(nil)
	task, err := a.DeleteScheduledTask(0)
	if err != nil {
		t.Error(err)
	}
	if task.Command != command {
		t.Errorf("expected command %s, got %s", command, task.Command)
	}
}
