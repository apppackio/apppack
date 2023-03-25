package stacks

import (
	"fmt"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/apppackio/apppack/bridge"
	"github.com/apppackio/apppack/stringslice"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudformation"
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
	StackType() string
	Tags(name *string) []*cloudformation.Tag
	Capabilities() []*string
	TemplateURL(release *string) *string
	PostCreate(sess *session.Session) error
	PreDelete(sess *session.Session) error
	PostDelete(sess *session.Session, name *string) error
}

func CloudformationParametersToStruct(s Parameters, parameters []*cloudformation.Parameter) error {
	ref := reflect.ValueOf(s).Type().Elem()
	for _, param := range parameters {
		field, ok := ref.FieldByName(*param.ParameterKey)
		if !ok {
			logrus.WithFields(logrus.Fields{"name": *param.ParameterKey}).Debug("unable to match Parameter")
			continue
		}
		val := reflect.ValueOf(s).Elem().FieldByName(*param.ParameterKey)
		switch field.Type.Kind() {
		case reflect.String:
			val.SetString(*param.ParameterValue)
		case reflect.Bool:
			var trueVal string
			if field.Tag.Get("cfnbool") == "yesno" {
				trueVal = "yes"
			} else {
				trueVal = "enabled"
			}
			if *param.ParameterValue == trueVal {
				val.SetBool(true)
			} else {
				val.SetBool(false)
			}
		case reflect.Int:
			i, err := strconv.Atoi(*param.ParameterValue)
			if err != nil {
				return err
			}
			val.SetInt(int64(i))
		case reflect.Slice:
			val.Set(reflect.ValueOf(strings.Split(*param.ParameterValue, ",")))
		default:
			return fmt.Errorf("unable to convert parameter %s to field", *param.ParameterKey)
		}
	}
	return nil
}

func StructToCloudformationParameters(s Parameters) ([]*cloudformation.Parameter, error) {
	var params []*cloudformation.Parameter
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
			var trueVal string
			var falseVal string
			param = &cloudformation.Parameter{ParameterKey: aws.String(field.Name)}
			if field.Tag.Get("cfnbool") == "yesno" {
				trueVal = "yes"
				falseVal = "no"
			} else {
				trueVal = "enabled"
				falseVal = "disabled"
			}
			if f.Bool() {
				param.ParameterValue = &trueVal
			} else {
				param.ParameterValue = &falseVal
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
			val := f.Interface().([]string)
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

// ExportParameters converts the parameters back into a list of cloudformation parameters
func ExportParameters(parameters Parameters, sess *session.Session, name *string) ([]*cloudformation.Parameter, error) {
	if err := parameters.SetInternalFields(sess, name); err != nil {
		return nil, err
	}
	return parameters.ToCloudFormationParameters()
}

// PruneUnsupportedParameters removes parameters that are not supported by the current stack version
func PruneUnsupportedParameters(supportedParameters []*cloudformation.Parameter, desiredParameters []*cloudformation.Parameter) []*cloudformation.Parameter {
	var supportedParameterNames []string
	for _, param := range supportedParameters {
		supportedParameterNames = append(supportedParameterNames, *param.ParameterKey)
	}
	var prunedParameters []*cloudformation.Parameter
	for _, param := range desiredParameters {
		if stringslice.Contains(*param.ParameterKey, supportedParameterNames) {
			prunedParameters = append(prunedParameters, param)
		} else {
			logrus.WithFields(logrus.Fields{"name": *param.ParameterKey}).Debug("parameter not supported by stack")
		}
	}
	return prunedParameters
}

func LoadStackFromCloudformation(sess *session.Session, stack Stack, name *string) error {
	cfnStackName := stack.StackName(name)
	cfnStack, err := bridge.GetStack(sess, *cfnStackName)
	if err != nil {
		return err
	}
	stack.SetStack(cfnStack)
	return stack.GetParameters().Import(cfnStack.Parameters)
}

// CreateStack creates a Cloudformation stack and waits for it to be created
func CreateStack(sess *session.Session, s Stack, name, release *string) error {
	params, err := ExportParameters(s.GetParameters(), sess, name)
	if err != nil {
		return err
	}
	cfnStack, err := CreateStackAndWait(sess, &cloudformation.CreateStackInput{
		StackName:    s.StackName(name),
		Parameters:   params,
		Capabilities: s.Capabilities(),
		Tags:         s.Tags(name),
		TemplateURL:  s.TemplateURL(release),
	})
	if err != nil {
		return err
	}
	if *cfnStack.StackStatus != "CREATE_COMPLETE" {
		return fmt.Errorf("stack creation failed: %s", *cfnStack.StackStatus)
	}
	s.SetStack(cfnStack)
	s.PostCreate(sess)
	return nil
}

// ModifyStack modifies a Cloudformation stack and waits for it to finish
func ModifyStack(sess *session.Session, s Stack, name *string) error {
	params, err := ExportParameters(s.GetParameters(), sess, name)
	if err != nil {
		return err
	}
	params = PruneUnsupportedParameters(s.GetStack().Parameters, params)
	cfnStack, err := UpdateStackAndWait(sess, &cloudformation.UpdateStackInput{
		StackName:           s.GetStack().StackName,
		Parameters:          params,
		UsePreviousTemplate: aws.Bool(true),
		Capabilities:        s.Capabilities(),
	})
	if err != nil {
		return err
	}
	if *cfnStack.StackStatus != "UPDATE_COMPLETE" {
		return fmt.Errorf("stack update failed: %s", *cfnStack.StackStatus)
	}
	return nil
}

// UpdateStack updates a Cloudformation stack and waits for it to finish
func UpdateStack(sess *session.Session, s Stack, name, release *string) error {
	params, err := ExportParameters(s.GetParameters(), sess, name)
	if err != nil {
		return err
	}
	params = PruneUnsupportedParameters(s.GetStack().Parameters, params)
	cfnStack, err := UpdateStackAndWait(sess, &cloudformation.UpdateStackInput{
		StackName:    s.GetStack().StackName,
		Parameters:   params,
		Capabilities: s.Capabilities(),
		TemplateURL:  s.TemplateURL(release),
	})
	if err != nil {
		return err
	}
	if *cfnStack.StackStatus != "UPDATE_COMPLETE" {
		return fmt.Errorf("stack update failed: %s", *cfnStack.StackStatus)
	}
	return nil
}

func CreateStackChangeset(sess *session.Session, s Stack, name, release *string) (string, error) {
	params, err := ExportParameters(s.GetParameters(), sess, name)
	if err != nil {
		return "", err
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
		return "", err
	}
	url := fmt.Sprintf("https://%s.console.aws.amazon.com/cloudformation/home#/stacks/changesets/changes?stackId=%s&changeSetId=%s", *sess.Config.Region, url.QueryEscape(*out.StackId), url.QueryEscape(*out.ChangeSetId))
	return url, nil
}

func ModifyStackChangeset(sess *session.Session, s Stack, name *string) (string, error) {
	params, err := ExportParameters(s.GetParameters(), sess, name)
	if err != nil {
		return "", err
	}
	type_ := "UPDATE"
	changeSetName := fmt.Sprintf("%s-%d", strings.ToLower(type_), int32(time.Now().Unix()))
	input := &cloudformation.CreateChangeSetInput{
		ChangeSetType:       &type_,
		ChangeSetName:       &changeSetName,
		StackName:           s.GetStack().StackName,
		UsePreviousTemplate: aws.Bool(true),
		Parameters:          params,
		Capabilities:        s.Capabilities(),
	}
	out, err := CreateChangeSetAndWait(sess, input)
	if err != nil {
		return "", err
	}
	url := fmt.Sprintf("https://%s.console.aws.amazon.com/cloudformation/home#/stacks/changesets/changes?stackId=%s&changeSetId=%s", *sess.Config.Region, url.QueryEscape(*out.StackId), url.QueryEscape(*out.ChangeSetId))
	return url, nil
}

func UpdateStackChangeset(sess *session.Session, s Stack, name, release *string) (string, error) {
	params, err := ExportParameters(s.GetParameters(), sess, name)
	if err != nil {
		return "", err
	}
	type_ := "UPDATE"
	changeSetName := fmt.Sprintf("%s-%d", strings.ToLower(type_), int32(time.Now().Unix()))
	input := &cloudformation.CreateChangeSetInput{
		ChangeSetType: &type_,
		ChangeSetName: &changeSetName,
		StackName:     s.GetStack().StackName,
		TemplateURL:   s.TemplateURL(release),
		Parameters:    params,
		Capabilities:  s.Capabilities(),
	}
	out, err := CreateChangeSetAndWait(sess, input)
	if err != nil {
		return "", err
	}
	url := fmt.Sprintf("https://%s.console.aws.amazon.com/cloudformation/home#/stacks/changesets/changes?stackId=%s&changeSetId=%s", *sess.Config.Region, url.QueryEscape(*out.StackId), url.QueryEscape(*out.ChangeSetId))
	return url, nil
}
