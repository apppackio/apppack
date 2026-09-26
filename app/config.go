package app

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	"github.com/aws/aws-sdk-go-v2/service/ssm"
	ssmtypes "github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"github.com/juju/ansiterm"
	"github.com/sirupsen/logrus"
)

type ConfigVariable struct {
	Name          string
	Value         string
	Managed       bool
	parameterName string
}

// LoadManaged loads the Managed value for the ConfigVariable from SSM tags
func (v *ConfigVariable) LoadManaged(ssmListTagsForResource func(*ssm.ListTagsForResourceInput) (*ssm.ListTagsForResourceOutput, error)) error {
	logrus.WithFields(logrus.Fields{"parameter": v.parameterName}).Debug("loading parameter tag")

	resourceType := ssmtypes.ResourceTypeForTaggingParameter
	resp, err := ssmListTagsForResource(&ssm.ListTagsForResourceInput{
		ResourceId:   &v.parameterName,
		ResourceType: resourceType,
	})
	if err != nil {
		return err
	}

	for _, tag := range resp.TagList {
		if *tag.Key == "aws:cloudformation:stack-id" || *tag.Key == "apppack:cloudformation:stack-id" {
			v.Managed = true

			return nil
		}
	}

	v.Managed = false

	return nil
}

type ConfigVariables []*ConfigVariable

// NewConfigVariables creates a new AppConfigVariables from the provided SSM parameters
func NewConfigVariables(parameters []ssmtypes.Parameter) ConfigVariables {
	var configVars ConfigVariables

	for _, parameter := range parameters {
		parts := strings.Split(*parameter.Name, "/")
		name := parts[len(parts)-1]
		configVars = append(configVars, &ConfigVariable{
			Name:          name,
			Value:         *parameter.Value,
			parameterName: *parameter.Name,
		})
	}

	sort.Slice(configVars, func(i, j int) bool {
		return configVars[i].Name < configVars[j].Name
	})

	return configVars
}

// Transform runs the provided function on each config variable
func (a *ConfigVariables) Transform(transformer func(*ConfigVariable) error) error {
	for _, configVar := range *a {
		if err := transformer(configVar); err != nil {
			return err
		}
	}

	return nil
}

// ToJSON returns a JSON representation of the config variables
func (a *ConfigVariables) ToJSON() (*bytes.Buffer, error) {
	results := map[string]string{}

	for _, configVar := range *a {
		results[configVar.Name] = configVar.Value
	}

	return toJSON(results)
}

// ToJSONUnmanaged returns a JSON representation of the unmanaged config variables
func (a *ConfigVariables) ToJSONUnmanaged() (*bytes.Buffer, error) {
	results := map[string]string{}

	for _, configVar := range *a {
		if configVar.Managed {
			continue
		}

		results[configVar.Name] = configVar.Value
	}

	return toJSON(results)
}

// printRow prints a single row of the table to the TabWriter
func printRow(w *ansiterm.TabWriter, name, value string) {
	w.SetForeground(ansiterm.Green)
	_, _ = fmt.Fprintf(w, "%s:", name)
	w.SetForeground(ansiterm.Default)
	_, _ = fmt.Fprintf(w, "\t%s\n", value)
}

// ToConsole prints the config vars to the console via the TabWriter
func (a *ConfigVariables) ToConsole(w *ansiterm.TabWriter) {
	for _, configVar := range *a {
		printRow(w, configVar.Name, configVar.Value)
	}
}

// GetParametersByPathFunc is the SSM call ConfigKeys depends on, extracted so
// it can be tested without an AWS client.
type GetParametersByPathFunc func(*ssm.GetParametersByPathInput) (*ssm.GetParametersByPathOutput, error)

// ConfigKeys returns the names of the config variables under prefix WITHOUT
// decrypting any values.
//
// This exists separately from SsmParameters because that function sets
// WithDecryption: true and returns plaintext secrets. `apppack diagnose` may
// see which variables are defined but never their values, so it calls this.
// SecureString values come back as ciphertext here and are discarded without
// ever being returned to a caller.
func ConfigKeys(get GetParametersByPathFunc, prefix string) ([]string, error) {
	withDecryption := false
	keys := []string{}

	input := ssm.GetParametersByPathInput{
		Path:           &prefix,
		WithDecryption: &withDecryption,
	}

	for {
		resp, err := get(&input)
		if err != nil {
			return nil, err
		}

		for i := range resp.Parameters {
			parts := strings.Split(*resp.Parameters[i].Name, "/")
			keys = append(keys, parts[len(parts)-1])
		}

		if resp.NextToken == nil {
			break
		}

		input.NextToken = resp.NextToken
	}

	sort.Strings(keys)

	return keys, nil
}
