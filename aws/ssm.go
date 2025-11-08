package aws

import (
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ssm"
)

type AWSInterface interface {
	GetParameter(input *ssm.GetParameterInput) (*string, error)
	PutParameter(input *ssm.PutParameterInput) error
	ValidateEventbridgeCron(schedule string) error
}

type AWS struct {
	session *session.Session
}

func New(sess *session.Session) *AWS {
	return &AWS{
		session: sess,
	}
}

func (a *AWS) GetParameter(input *ssm.GetParameterInput) (*string, error) {
	ssmSvc := ssm.New(a.session)

	parameterOutput, err := ssmSvc.GetParameter(input)
	if err != nil {
		return nil, err
	}

	return parameterOutput.Parameter.Value, nil
}

func (a *AWS) PutParameter(input *ssm.PutParameterInput) error {
	ssmSvc := ssm.New(a.session)
	_, err := ssmSvc.PutParameter(input)

	return err
}
