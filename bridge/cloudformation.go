package bridge

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudformation"
	"github.com/sirupsen/logrus"
)

func CloudformationParametersToStruct(s interface{}, parameters []*cloudformation.Parameter) error {
	ref := reflect.ValueOf(s).Elem()
	for _, param := range parameters {
		field := ref.FieldByName(*param.ParameterKey)
		if !field.IsValid() {
			logrus.Debug("unable to match Parameter name %s", *param.ParameterKey)
			continue
		}
		switch field.Kind() {
		case reflect.String:
			field.SetString(*param.ParameterValue)
		case reflect.Bool:
			if *param.ParameterValue == "enabled" {
				field.SetBool(true)
			} else {
				field.SetBool(false)
			}
		case reflect.Int:
			i, err := strconv.Atoi(*param.ParameterValue)
			if err != nil {
				return err
			}
			field.SetInt(int64(i))
		case reflect.Slice:
			field.Set(reflect.ValueOf(strings.Split(*param.ParameterValue, ",")))
		default:
			return fmt.Errorf("unable to convert parameter %s to field", *param.ParameterKey)
		}
	}
	return nil
}

func StructToCloudformationParameters(s interface{}) ([]*cloudformation.Parameter, error) {
	params := []*cloudformation.Parameter{}
	ref := reflect.ValueOf(s).Elem()
	if ref.Kind() != reflect.Struct {
		return nil, fmt.Errorf("expected struct, got %s", ref.Kind())
	}
	fields := reflect.VisibleFields(ref.Type())
	for i, field := range fields {
		f := reflect.ValueOf(s).Elem().Field(i)
		var param *cloudformation.Parameter
		switch field.Type.Kind() {
		case reflect.String:
			param = &cloudformation.Parameter{
				ParameterKey:   aws.String(field.Name),
				ParameterValue: aws.String(f.String()),
			}
		case reflect.Bool:
			var val string
			if f.Bool() {
				val = "enabled"
			} else {
				val = "disabled"
			}
			param = &cloudformation.Parameter{
				ParameterKey:   aws.String(field.Name),
				ParameterValue: &val,
			}
		case reflect.Int:
			val := f.Int()
			param = &cloudformation.Parameter{
				ParameterKey:   aws.String(field.Name),
				ParameterValue: aws.String(strconv.Itoa(int(val))),
			}
		case reflect.Slice:
			if f.Type().Elem().Kind() != reflect.String {
				return nil, fmt.Errorf("%s is not a slice of strings", field.Name)
			}
			val := []string{}
			for i := 0; i < f.Len(); i++ {
				val = append(val, f.Index(i).String())
			}
			param = &cloudformation.Parameter{
				ParameterKey:   aws.String(field.Name),
				ParameterValue: aws.String(strings.Join(val, ",")),
			}
		default:
			return nil, fmt.Errorf("unable to convert field %s to parameter", field.Name)
		}
		params = append(params, param)
	}
	return params, nil
}

// stackExists checks if a named Cfn Stack already exists in the region
func StackExists(sess *session.Session, stackName string) (*bool, error) {
	cfnSvc := cloudformation.New(sess)
	stackOutput, err := cfnSvc.DescribeStacks(&cloudformation.DescribeStacksInput{
		StackName: &stackName,
	})
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
	exists = len(stackOutput.Stacks) > 0 && *stackOutput.Stacks[0].StackStatus != cloudformation.StackStatusDeleteComplete
	return &exists, nil
}
