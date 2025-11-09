package app_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/apppackio/apppack/app"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	ssmtypes "github.com/aws/aws-sdk-go-v2/service/ssm/types"
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
	t.Parallel()

	a := &app.App{
		Name: "test",
		AWS:  &MockAWS{},
	}
	a.AWS.(*MockAWS).On(
		"GetParameter",
		&ssm.GetParameterInput{Name: aws.String("/apppack/apps/test/scheduled-tasks")},
	).Return(nil, errors.New("parameter not found"))

	tasks, err := a.ScheduledTasks()
	if err != nil {
		t.Error(err)
	}

	if len(tasks) != 0 {
		t.Errorf("expected 0 tasks, got %d", len(tasks))
	}
}

func TestScheduledTasksCreate(t *testing.T) {
	t.Parallel()

	a := &app.App{
		Name: "test",
		AWS:  &MockAWS{},
	}
	parameterName := "/apppack/apps/test/scheduled-tasks"
	a.AWS.(*MockAWS).On(
		"GetParameter",
		&ssm.GetParameterInput{Name: &parameterName},
	).Return(nil, errors.New("parameter not found"))

	schedule := "0/10 * * * ? *"
	a.AWS.(*MockAWS).On(
		"ValidateEventbridgeCron",
		schedule,
	).Return(nil)

	command := "echo hello"
	parameterType := ssmtypes.ParameterTypeString
	a.AWS.(*MockAWS).On(
		"PutParameter",
		&ssm.PutParameterInput{
			Name:      &parameterName,
			Value:     aws.String(fmt.Sprintf("[{\"schedule\":%q,\"command\":%q}]", schedule, command)),
			Type:      parameterType,
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
	t.Parallel()

	a := &app.App{
		Name: "test",
		AWS:  &MockAWS{},
	}
	parameterName := "/apppack/apps/test/scheduled-tasks"
	schedule := "0/10 * * * ? *"
	command := "echo hello"
	a.AWS.(*MockAWS).On(
		"GetParameter",
		&ssm.GetParameterInput{Name: &parameterName},
	).Return(aws.String(fmt.Sprintf("[{\"schedule\":%q,\"command\":%q}]", schedule, command)), nil)
	parameterType := ssmtypes.ParameterTypeString
	a.AWS.(*MockAWS).On(
		"PutParameter",
		&ssm.PutParameterInput{
			Name:      &parameterName,
			Value:     aws.String("[]"),
			Type:      parameterType,
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

func TestScheduledTasksDeleteEmpty(t *testing.T) {
	t.Parallel()

	a := &app.App{
		Name: "test",
		AWS:  &MockAWS{},
	}
	parameterName := "/apppack/apps/test/scheduled-tasks"
	a.AWS.(*MockAWS).On(
		"GetParameter",
		&ssm.GetParameterInput{Name: &parameterName},
	).Return(aws.String("[]"), nil)

	_, err := a.DeleteScheduledTask(0)
	if err == nil {
		t.Error("expected error trying to delete from empty list")
	}
}
