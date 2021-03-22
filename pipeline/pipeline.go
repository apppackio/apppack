package pipeline

import (
	"encoding/json"
	"fmt"

	"github.com/apppackio/apppack/app"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ssm"
)

type Pipeline struct {
	App app.App
}

// ReviewApp is a representation of a AppPack app
type ReviewApp struct {
	PullRequest string `json:"pull_request"`
	Status      string `json:"status"`
	Branch      string `json:"branch"`
}

func Init(name string) (*Pipeline, error) {
	p := Pipeline{}
	a, err := app.Init(name)
	if err != nil {
		return nil, err
	}
	p.App = *a
	return &p, nil
}

// updateJsonVal takes a JSON string and adds/updates a given `key` to `value` returning the new JSON
func updateJsonVal(original string, key string, value string) (*string, error) {
	data := make(map[string]interface{})
	err := json.Unmarshal([]byte(original), &data)
	if err != nil {
		return nil, err
	}
	data[key] = value
	new, err := json.Marshal(&data)
	if err != nil {
		return nil, err
	}
	ret := string(new)
	return &ret, nil
}

func (p Pipeline) SetReviewAppStatus(pullRequestNumber string, status string) error {
	ssmSvc := ssm.New(p.App.Session)
	parameterName := fmt.Sprintf("/apppack/pipelines/%s/review-apps/pr/%s", p.App.Name, pullRequestNumber)
	parameterOutput, err := ssmSvc.GetParameter(&ssm.GetParameterInput{
		Name: &parameterName,
	})
	if err != nil {
		return err
	}
	j, err := updateJsonVal(*parameterOutput.Parameter.Value, "status", status)
	if err != nil {
		return err
	}
	ssmSvc.PutParameter(&ssm.PutParameterInput{
		Name:      &parameterName,
		Type:      aws.String("String"),
		Overwrite: aws.Bool(true),
		Value:     j,
	})
	return nil
}

func (p *Pipeline) GetReviewApps() ([]*ReviewApp, error) {
	parameters, err := app.SsmParameters(p.App.Session, fmt.Sprintf("/apppack/pipelines/%s/review-apps/pr/", p.App.Name))
	if err != nil {
		return nil, err
	}
	var reviewApps []*ReviewApp
	for _, parameter := range parameters {
		r := ReviewApp{}
		err = json.Unmarshal([]byte(*parameter.Value), &r)
		if err != nil {
			return nil, err
		}
		reviewApps = append(reviewApps, &r)
	}
	return reviewApps, nil
}
