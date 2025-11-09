package stacks

import (
	"errors"
	"fmt"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/apppackio/apppack/bridge"
	"github.com/apppackio/apppack/stringslice"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"github.com/sirupsen/logrus"
	"github.com/spf13/pflag"
)

var ErrStackCreationFailed = errors.New("stack creation failed")

type Parameters interface {
	Import([]types.Parameter) error
	ToCloudFormationParameters() ([]types.Parameter, error)
	SetInternalFields(cfg aws.Config, name *string) error
}
type Stack interface {
	GetParameters() Parameters
	GetStack() *types.Stack
	SetStack(stack *types.Stack)
	UpdateFromFlags(flags *pflag.FlagSet) error
	AskQuestions(cfg aws.Config) error
	StackName(name *string) *string
	StackType() string
	Tags(name *string) []types.Tag
	Capabilities() []types.Capability
	TemplateURL(release *string) *string
	PostCreate(cfg aws.Config) error
	PreDelete(cfg aws.Config) error
	PostDelete(cfg aws.Config, name *string) error
}

func CloudformationParametersToStruct(s Parameters, parameters []types.Parameter) error {
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
				trueVal = Enabled
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

func StructToCloudformationParameters(s Parameters) ([]types.Parameter, error) {
	var params []types.Parameter

	ref := reflect.ValueOf(s).Elem()
	if ref.Kind() != reflect.Struct {
		return nil, fmt.Errorf("expected struct, got %s", ref.Kind())
	}

	fields := reflect.VisibleFields(ref.Type())
	for i, field := range fields {
		f := reflect.ValueOf(s).Elem().Field(i)

		var param types.Parameter

		switch field.Type.Kind() {
		case reflect.String:
			param = types.Parameter{
				ParameterKey:   aws.String(field.Name),
				ParameterValue: aws.String(f.String()),
			}
		case reflect.Bool:
			var trueVal string

			var falseVal string

			param = types.Parameter{ParameterKey: aws.String(field.Name)}

			if field.Tag.Get("cfnbool") == "yesno" {
				trueVal = "yes"
				falseVal = "no"
			} else {
				trueVal = Enabled
				falseVal = "disabled"
			}

			if f.Bool() {
				param.ParameterValue = &trueVal
			} else {
				param.ParameterValue = &falseVal
			}

		case reflect.Int:
			val := f.Int()
			param = types.Parameter{
				ParameterKey:   aws.String(field.Name),
				ParameterValue: aws.String(strconv.Itoa(int(val))),
			}
		case reflect.Slice:
			if f.Type().Elem().Kind() != reflect.String {
				return nil, fmt.Errorf("%s is not a slice of strings", field.Name)
			}

			val := f.Interface().([]string)
			param = types.Parameter{
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
func ExportParameters(parameters Parameters, cfg aws.Config, name *string) ([]types.Parameter, error) {
	if err := parameters.SetInternalFields(cfg, name); err != nil {
		return nil, err
	}

	return parameters.ToCloudFormationParameters()
}

// PruneUnsupportedParameters removes parameters that are not supported by the current stack version
func PruneUnsupportedParameters(supportedParameters, desiredParameters []types.Parameter) []types.Parameter {
	var supportedParameterNames []string
	for _, param := range supportedParameters {
		supportedParameterNames = append(supportedParameterNames, *param.ParameterKey)
	}

	var prunedParameters []types.Parameter

	for _, param := range desiredParameters {
		if stringslice.Contains(*param.ParameterKey, supportedParameterNames) {
			prunedParameters = append(prunedParameters, param)
		} else {
			logrus.WithFields(logrus.Fields{"name": *param.ParameterKey}).Debug("parameter not supported by stack")
		}
	}

	return prunedParameters
}

func LoadStackFromCloudformation(cfg aws.Config, stack Stack, name *string) error {
	cfnStackName := stack.StackName(name)

	cfnStack, err := bridge.GetStack(cfg, *cfnStackName)
	if err != nil {
		return err
	}

	stack.SetStack(cfnStack)

	return stack.GetParameters().Import(cfnStack.Parameters)
}

// CreateStack creates a Cloudformation stack and waits for it to be created
func CreateStack(cfg aws.Config, s Stack, name, release *string) error {
	params, err := ExportParameters(s.GetParameters(), cfg, name)
	if err != nil {
		return err
	}

	cfnStack, err := CreateStackAndWait(cfg, &cloudformation.CreateStackInput{
		StackName:    s.StackName(name),
		Parameters:   params,
		Capabilities: s.Capabilities(),
		Tags:         s.Tags(name),
		TemplateURL:  s.TemplateURL(release),
	})
	if err != nil {
		return err
	}

	if cfnStack.StackStatus != types.StackStatusCreateComplete {
		return ErrStackCreationFailed
	}

	s.SetStack(cfnStack)

	return s.PostCreate(cfg)
}

// ModifyStack modifies a Cloudformation stack and waits for it to finish
func ModifyStack(cfg aws.Config, s Stack, name *string) error {
	params, err := ExportParameters(s.GetParameters(), cfg, name)
	if err != nil {
		return err
	}

	params = PruneUnsupportedParameters(s.GetStack().Parameters, params)

	cfnStack, err := UpdateStackAndWait(cfg, &cloudformation.UpdateStackInput{
		StackName:           s.GetStack().StackName,
		Parameters:          params,
		UsePreviousTemplate: aws.Bool(true),
		Capabilities:        s.Capabilities(),
	})
	if err != nil {
		return err
	}

	if cfnStack.StackStatus != types.StackStatusUpdateComplete {
		return fmt.Errorf("stack update failed: %s", cfnStack.StackStatus)
	}

	return nil
}

// UpdateStack updates a Cloudformation stack and waits for it to finish
func UpdateStack(cfg aws.Config, s Stack, name, release *string) error {
	params, err := ExportParameters(s.GetParameters(), cfg, name)
	if err != nil {
		return err
	}

	params = PruneUnsupportedParameters(s.GetStack().Parameters, params)

	cfnStack, err := UpdateStackAndWait(cfg, &cloudformation.UpdateStackInput{
		StackName:    s.GetStack().StackName,
		Parameters:   params,
		Capabilities: s.Capabilities(),
		TemplateURL:  s.TemplateURL(release),
	})
	if err != nil {
		return err
	}

	if cfnStack.StackStatus != types.StackStatusUpdateComplete {
		return fmt.Errorf("stack update failed: %s", cfnStack.StackStatus)
	}

	return nil
}

func CreateStackChangeset(cfg aws.Config, s Stack, name, release *string) (string, error) {
	params, err := ExportParameters(s.GetParameters(), cfg, name)
	if err != nil {
		return "", err
	}

	changeSetType := types.ChangeSetTypeCreate
	changeSetName := fmt.Sprintf("%s-%d", strings.ToLower(string(changeSetType)), time.Now().Unix())
	input := &cloudformation.CreateChangeSetInput{
		ChangeSetType: changeSetType,
		ChangeSetName: &changeSetName,
		StackName:     s.StackName(name),
		Parameters:    params,
		Capabilities:  s.Capabilities(),
		Tags:          s.Tags(name),
		TemplateURL:   s.TemplateURL(release),
	}

	out, err := CreateChangeSetAndWait(cfg, input)
	if err != nil {
		return "", err
	}

	url := fmt.Sprintf(
		"https://%s.console.aws.amazon.com/cloudformation/home#/stacks/changesets/changes?stackId=%s&changeSetId=%s",
		cfg.Region,
		url.QueryEscape(*out.StackId),
		url.QueryEscape(*out.ChangeSetId),
	)

	return url, nil
}

func ModifyStackChangeset(cfg aws.Config, s Stack, name *string) (string, error) {
	params, err := ExportParameters(s.GetParameters(), cfg, name)
	if err != nil {
		return "", err
	}

	changeSetType := types.ChangeSetTypeUpdate
	changeSetName := fmt.Sprintf("%s-%d", strings.ToLower(string(changeSetType)), time.Now().Unix())
	input := &cloudformation.CreateChangeSetInput{
		ChangeSetType:       changeSetType,
		ChangeSetName:       &changeSetName,
		StackName:           s.GetStack().StackName,
		UsePreviousTemplate: aws.Bool(true),
		Parameters:          params,
		Capabilities:        s.Capabilities(),
	}

	out, err := CreateChangeSetAndWait(cfg, input)
	if err != nil {
		return "", err
	}

	url := fmt.Sprintf(
		"https://%s.console.aws.amazon.com/cloudformation/home#/stacks/changesets/changes?stackId=%s&changeSetId=%s",
		cfg.Region,
		url.QueryEscape(*out.StackId),
		url.QueryEscape(*out.ChangeSetId),
	)

	return url, nil
}

func UpdateStackChangeset(cfg aws.Config, s Stack, name, release *string) (string, error) {
	params, err := ExportParameters(s.GetParameters(), cfg, name)
	if err != nil {
		return "", err
	}

	changeSetType := types.ChangeSetTypeUpdate
	changeSetName := fmt.Sprintf("%s-%d", strings.ToLower(string(changeSetType)), time.Now().Unix())
	input := &cloudformation.CreateChangeSetInput{
		ChangeSetType: changeSetType,
		ChangeSetName: &changeSetName,
		StackName:     s.GetStack().StackName,
		TemplateURL:   s.TemplateURL(release),
		Parameters:    params,
		Capabilities:  s.Capabilities(),
	}

	out, err := CreateChangeSetAndWait(cfg, input)
	if err != nil {
		return "", err
	}

	url := fmt.Sprintf(
		"https://%s.console.aws.amazon.com/cloudformation/home#/stacks/changesets/changes?stackId=%s&changeSetId=%s",
		cfg.Region,
		url.QueryEscape(*out.StackId),
		url.QueryEscape(*out.ChangeSetId),
	)

	return url, nil
}
