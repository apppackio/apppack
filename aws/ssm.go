package aws

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
)

type Interface interface {
	GetParameter(input *ssm.GetParameterInput) (*string, error)
	PutParameter(input *ssm.PutParameterInput) error
	ValidateEventbridgeCron(schedule string) error
}

type AWS struct {
	cfg aws.Config
}

func New(cfg aws.Config) *AWS {
	return &AWS{
		cfg: cfg,
	}
}

func (a *AWS) GetParameter(input *ssm.GetParameterInput) (*string, error) {
	ssmSvc := ssm.NewFromConfig(a.cfg)

	parameterOutput, err := ssmSvc.GetParameter(context.Background(), input)
	if err != nil {
		return nil, err
	}

	return parameterOutput.Parameter.Value, nil
}

func (a *AWS) PutParameter(input *ssm.PutParameterInput) error {
	ssmSvc := ssm.NewFromConfig(a.cfg)
	_, err := ssmSvc.PutParameter(context.Background(), input)

	return err
}
