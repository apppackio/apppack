package stacks_test

import (
	"testing"

	"github.com/apppackio/apppack/stacks"
	"github.com/aws/aws-sdk-go/aws"
)

func TestCustomDomainStackName(t *testing.T) {
	t.Parallel()

	scenarios := []struct {
		input    string
		expected string
	}{
		{"example.com", "apppack-customdomain-example-com"},
		{"*.example.com", "apppack-customdomain-wildcard-example-com"},
	}

	stack := stacks.CustomDomainStack{}
	for _, s := range scenarios {
		actual := stack.StackName(aws.String(s.input))
		if *actual != s.expected {
			t.Errorf("Expected %s, got %s", s.expected, *actual)
		}
	}
}
