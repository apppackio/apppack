package stacks

import (
	"fmt"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudformation"
	"github.com/logrusorgru/aurora"
	"github.com/sirupsen/logrus"
	"github.com/spf13/pflag"
)

type Parameters interface {
	Import([]*cloudformation.Parameter) error
	ToCloudFormationParameters() ([]*cloudformation.Parameter, error)
	SetInternalFields(sess *session.Session, name *string) error
}
type Stack interface {
	GetParameters() Parameters
	GetStack() *cloudformation.Stack
	SetStack(stack *cloudformation.Stack)
	UpdateFromFlags(flags *pflag.FlagSet) error
	AskQuestions(sess *session.Session) error
	StackName(name *string) *string
	Tags(name *string) []*cloudformation.Tag
	Capabilities() []*string
	TemplateURL(release *string) *string
}

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

// LoadStack pulls in the parameters from cloudformation and applies any values set in the flags
func LoadStack(stack Stack, flags *pflag.FlagSet) error {
	cfnStack := stack.GetStack()
	if cfnStack != nil {
		if err := stack.GetParameters().Import(stack.GetStack().Parameters); err != nil {
			return err
		}
	}
	if err := stack.UpdateFromFlags(flags); err != nil {
		return err
	}
	return nil
}

// ExportParameters converts the parameters back into a list of cloudformation parameters
func ExportParameters(parameters Parameters, sess *session.Session, name *string) ([]*cloudformation.Parameter, error) {
	if err := parameters.SetInternalFields(sess, name); err != nil {
		return nil, err
	}
	return parameters.ToCloudFormationParameters()
}

// CreateStack creates a Cloudformation stack and waits for it to be created
func CreateStack(s Stack, sess *session.Session, name *string, release *string) error {
	params, err := ExportParameters(s.GetParameters(), sess, name)
	if err != nil {
		return err
	}
	stack, err := CreateStackAndWait(sess, &cloudformation.CreateStackInput{
		StackName:    s.StackName(name),
		Parameters:   params,
		Capabilities: s.Capabilities(),
		Tags:         s.Tags(name),
		TemplateURL:  s.TemplateURL(release),
	})
	s.SetStack(stack)
	return err
}

// ModifyStack modifies a Cloudformation stack and waits for it to finish
func ModifyStack(s Stack, sess *session.Session) error {
	params, err := ExportParameters(s.GetParameters(), sess, s.GetStack().StackName)
	if err != nil {
		return err
	}
	_, err = UpdateStackAndWait(sess, &cloudformation.UpdateStackInput{
		StackName:           s.GetStack().StackName,
		Parameters:          params,
		UsePreviousTemplate: aws.Bool(true),
		Capabilities:        s.GetStack().Capabilities,
	})
	return err
}

func CreateStackChangeset(s Stack, sess *session.Session, name *string, release *string) error {
	params, err := ExportParameters(s.GetParameters(), sess, s.GetStack().StackName)
	if err != nil {
		return err
	}
	type_ := "CREATE"
	changeSetName := fmt.Sprintf("%s-%d", strings.ToLower(type_), int32(time.Now().Unix()))
	input := &cloudformation.CreateChangeSetInput{
		ChangeSetType: &type_,
		ChangeSetName: &changeSetName,
		StackName:     s.StackName(name),
		Parameters:    params,
		Capabilities:  s.Capabilities(),
		Tags:          s.Tags(name),
		TemplateURL:   s.TemplateURL(release),
	}
	out, err := CreateChangeSetAndWait(sess, input)
	if err != nil {
		return err
	}
	fmt.Println("View changeset at", aurora.White(
		fmt.Sprintf("https://%s.console.aws.amazon.com/cloudformation/home#/stacks/changesets/changes?stackId=%s&changeSetId=%s", *sess.Config.Region, url.QueryEscape(*out.StackId), url.QueryEscape(*out.ChangeSetId)),
	))
	return nil
}

func ModifyStackChangeset(s Stack, sess *session.Session) error {
	params, err := ExportParameters(s.GetParameters(), sess, s.GetStack().StackName)
	if err != nil {
		return err
	}
	type_ := "UPDATE"
	changeSetName := fmt.Sprintf("%s-%d", strings.ToLower(type_), int32(time.Now().Unix()))
	input := &cloudformation.CreateChangeSetInput{
		ChangeSetType:       &type_,
		ChangeSetName:       &changeSetName,
		StackName:           s.GetStack().StackName,
		UsePreviousTemplate: aws.Bool(true),
		Parameters:          params,
		Capabilities:        s.GetStack().Capabilities,
	}
	out, err := CreateChangeSetAndWait(sess, input)
	if err != nil {
		return err
	}
	fmt.Println("View changeset at", aurora.White(
		fmt.Sprintf("https://%s.console.aws.amazon.com/cloudformation/home#/stacks/changesets/changes?stackId=%s&changeSetId=%s", *sess.Config.Region, url.QueryEscape(*out.StackId), url.QueryEscape(*out.ChangeSetId)),
	))
	return nil
}
