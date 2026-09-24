package app_test

import (
	"bytes"
	"errors"
	"testing"

	"github.com/apppackio/apppack/app"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	ssmtypes "github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"github.com/juju/ansiterm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var errMock = errors.New("mock error")

func TestConfigVariablesToJSON(t *testing.T) {
	t.Parallel()

	c := app.NewConfigVariables([]ssmtypes.Parameter{
		{Name: aws.String("/apppack/apps/myapp/FOO"), Value: aws.String("bar")},
	})

	js, err := c.ToJSON()
	if err != nil {
		t.Error(err)
	}

	expected := `{
  "FOO": "bar"
}`
	actual := js.String()

	if actual != expected {
		t.Errorf("expected %s, got %s", expected, actual)
	}
}

func TestConfigVariablesToJSONUnmanaged(t *testing.T) {
	t.Parallel()

	c := app.ConfigVariables([]*app.ConfigVariable{
		{Name: "FOO", Value: "bar", Managed: false},
		{Name: "BAZ", Value: "qux", Managed: true},
	})

	js, err := c.ToJSONUnmanaged()
	if err != nil {
		t.Error(err)
	}

	expected := `{
  "FOO": "bar"
}`
	actual := js.String()

	if actual != expected {
		t.Errorf("expected %s, got %s", expected, actual)
	}
}

func TestConfigVariablesToConsole(t *testing.T) {
	t.Parallel()

	c := app.NewConfigVariables([]ssmtypes.Parameter{
		{Name: aws.String("/apppack/apps/myapp/config/FOO"), Value: aws.String("bar")},
		{Name: aws.String("/apppack/apps/myapp/LONGERVARIABLEFOO"), Value: aws.String("baz")},
	})
	out := &bytes.Buffer{}
	w := ansiterm.NewTabWriter(out, 8, 8, 0, '\t', 0)
	c.ToConsole(w)

	expected := []byte("FOO:\t\t\tbar\nLONGERVARIABLEFOO:\tbaz\n")

	_ = w.Flush()

	actual := out.Bytes()
	if !bytes.Equal(actual, expected) {
		t.Errorf("expected %b, got %b", expected, actual)
	}
}

func TestConfigVariablesTransform(t *testing.T) {
	t.Parallel()

	c := app.NewConfigVariables([]ssmtypes.Parameter{
		{Name: aws.String("/apppack/apps/myapp/config/FOO"), Value: aws.String("bar")},
		{Name: aws.String("/apppack/apps/myapp/config/BAZ"), Value: aws.String("qux")},
	})
	transformedVals := map[string]string{
		"FOO": "newvalue_foo",
		"BAZ": "newvalue_qux",
	}

	err := c.Transform(func(v *app.ConfigVariable) error {
		v.Value = transformedVals[v.Name]

		return nil
	})
	if err != nil {
		t.Error(err)
	}

	for _, v := range c {
		if v.Value != transformedVals[v.Name] {
			t.Errorf("expected %s, got %s", transformedVals[v.Name], v.Value)
		}
	}

	err = c.Transform(func(*app.ConfigVariable) error { return errMock })

	if !errors.Is(err, errMock) {
		t.Errorf("expected %s, got %s", errMock, err)
	}
}

func TestConfigVariableLoadManaged(t *testing.T) {
	t.Parallel()

	managedVar := app.NewConfigVariables([]ssmtypes.Parameter{
		{Name: aws.String("/apppack/apps/myapp/config/FOO"), Value: aws.String("bar")},
	})[0]

	unmanagedVar := managedVar

	managedFunc := func(*ssm.ListTagsForResourceInput) (*ssm.ListTagsForResourceOutput, error) {
		return &ssm.ListTagsForResourceOutput{
			TagList: []ssmtypes.Tag{{Key: aws.String("aws:cloudformation:stack-id"), Value: aws.String("stackid")}},
		}, nil
	}

	unmanagedFunc := func(*ssm.ListTagsForResourceInput) (*ssm.ListTagsForResourceOutput, error) {
		return &ssm.ListTagsForResourceOutput{TagList: []ssmtypes.Tag{}}, nil
	}

	errorFunc := func(*ssm.ListTagsForResourceInput) (*ssm.ListTagsForResourceOutput, error) {
		return nil, errMock
	}

	scenarios := []struct {
		cVar     *app.ConfigVariable
		f        func(*ssm.ListTagsForResourceInput) (*ssm.ListTagsForResourceOutput, error)
		expected bool
	}{
		{cVar: managedVar, f: managedFunc, expected: true},
		{cVar: unmanagedVar, f: unmanagedFunc, expected: false},
	}

	for _, s := range scenarios {
		err := s.cVar.LoadManaged(s.f)
		if err != nil {
			t.Error(err)
		}

		if s.cVar.Managed != s.expected {
			t.Errorf("expected %t, got %t", s.expected, s.cVar.Managed)
		}
	}

	err := managedVar.LoadManaged(errorFunc)
	if !errors.Is(err, errMock) {
		t.Errorf("expected %s, got %s", errMock, err)
	}
}

// Review Focus: the diagnose feature must never receive plaintext secrets.
// This test is the enforcement point for spec invariant 2.
func TestConfigKeysNeverRequestsDecryption(t *testing.T) {
	t.Parallel()

	var gotInputs []*ssm.GetParametersByPathInput

	get := func(in *ssm.GetParametersByPathInput) (*ssm.GetParametersByPathOutput, error) {
		gotInputs = append(gotInputs, in)

		return &ssm.GetParametersByPathOutput{
			Parameters: []ssmtypes.Parameter{
				{Name: aws.String("/apppack/apps/myapp/config/DATABASE_URL"), Value: aws.String("ciphertext")},
				{Name: aws.String("/apppack/apps/myapp/config/SECRET_KEY"), Value: aws.String("ciphertext")},
			},
		}, nil
	}

	keys, err := app.ConfigKeys(get, "/apppack/apps/myapp/config/")
	require.NoError(t, err)
	assert.Equal(t, []string{"DATABASE_URL", "SECRET_KEY"}, keys)

	require.Len(t, gotInputs, 1)
	require.NotNil(t, gotInputs[0].WithDecryption)
	assert.False(t, *gotInputs[0].WithDecryption, "diagnose must never request decrypted SSM values")
}

func TestConfigKeysPaginates(t *testing.T) {
	t.Parallel()

	calls := 0

	get := func(in *ssm.GetParametersByPathInput) (*ssm.GetParametersByPathOutput, error) {
		calls++
		if calls == 1 {
			return &ssm.GetParametersByPathOutput{
				Parameters: []ssmtypes.Parameter{
					{Name: aws.String("/apppack/apps/myapp/config/A"), Value: aws.String("x")},
				},
				NextToken: aws.String("more"),
			}, nil
		}

		return &ssm.GetParametersByPathOutput{
			Parameters: []ssmtypes.Parameter{
				{Name: aws.String("/apppack/apps/myapp/config/B"), Value: aws.String("x")},
			},
		}, nil
	}

	keys, err := app.ConfigKeys(get, "/apppack/apps/myapp/config/")
	require.NoError(t, err)
	assert.Equal(t, []string{"A", "B"}, keys)
	assert.Equal(t, 2, calls)
}

func TestConfigKeysPropagatesError(t *testing.T) {
	t.Parallel()

	get := func(*ssm.GetParametersByPathInput) (*ssm.GetParametersByPathOutput, error) {
		return nil, errMock
	}

	_, err := app.ConfigKeys(get, "/apppack/apps/myapp/config/")
	require.ErrorIs(t, err, errMock)
}
