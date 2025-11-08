package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ssm"
)

type ScheduledTask struct {
	Schedule string `json:"schedule"`
	Command  string `json:"command"`
}

// ScheduledTasks lists scheduled tasks for the app
func (a *App) ScheduledTasks() ([]*ScheduledTask, error) {
	parameterName := fmt.Sprintf("/apppack/apps/%s/scheduled-tasks", a.Name)
	value, err := a.AWS.GetParameter(&ssm.GetParameterInput{
		Name: &parameterName,
	})

	var tasks []*ScheduledTask

	if err != nil {
		tasks = []*ScheduledTask{}
	} else if err = json.Unmarshal([]byte(*value), &tasks); err != nil {
		return nil, err
	}

	return tasks, nil
}

// CreateScheduledTask adds a scheduled task for the app
func (a *App) CreateScheduledTask(schedule, command string) ([]*ScheduledTask, error) {
	if err := a.AWS.ValidateEventbridgeCron(schedule); err != nil {
		return nil, err
	}

	tasks, err := a.ScheduledTasks()
	if err != nil {
		return nil, err
	}

	tasks = append(tasks, &ScheduledTask{
		Schedule: strings.TrimSpace(schedule),
		Command:  strings.TrimSpace(command),
	})
	// avoid exceeding AWS quota
	tasksBySchedule := map[string][]string{}
	for _, task := range tasks {
		tasksBySchedule[task.Schedule] = append(tasksBySchedule[task.Schedule], task.Command)
	}

	for schedule, commands := range tasksBySchedule {
		if len(commands) > 5 {
			return nil, fmt.Errorf("AWS quota limits a single schedule to no more than 5 tasks (%s)", schedule)
		}
	}

	tasksBytes, err := json.Marshal(tasks)
	if err != nil {
		return nil, err
	}

	parameterName := fmt.Sprintf("/apppack/apps/%s/scheduled-tasks", a.Name)

	err = a.AWS.PutParameter(&ssm.PutParameterInput{
		Name:      &parameterName,
		Value:     aws.String(string(tasksBytes)),
		Overwrite: aws.Bool(true),
		Type:      aws.String("String"),
	})
	if err != nil {
		return nil, err
	}

	return tasks, nil
}

// DeleteScheduledTask deletes the scheduled task at the given index
func (a *App) DeleteScheduledTask(idx int) (*ScheduledTask, error) {
	tasks, err := a.ScheduledTasks()
	if err != nil {
		return nil, err
	}

	if idx >= len(tasks) || idx < 0 {
		return nil, errors.New("invalid index for task to delete")
	}

	taskToDelete := tasks[idx]
	tasks = append(tasks[:idx], tasks[idx+1:]...)

	tasksBytes, err := json.Marshal(tasks)
	if err != nil {
		return nil, err
	}

	parameterName := fmt.Sprintf("/apppack/apps/%s/scheduled-tasks", a.Name)

	err = a.AWS.PutParameter(&ssm.PutParameterInput{
		Name:      &parameterName,
		Value:     aws.String(string(tasksBytes)),
		Overwrite: aws.Bool(true),
		Type:      aws.String("String"),
	})
	if err != nil {
		return nil, err
	}

	return taskToDelete, nil
}
