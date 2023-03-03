package stacks

import (
	"testing"

	"github.com/aws/aws-sdk-go/aws"
)

func TestCustomDomainStackName(t *testing.T) {
	stack := CustomDomainStack{}
	actual := stack.StackName(aws.String("example.com"))
	expected := "apppack-customdomain-example-com"
	if *actual != expected {
		t.Errorf("Expected %s, got %s", expected, *actual)
	}

	actual = stack.StackName(aws.String("*.example.com"))
	expected = "apppack-customdomain-wildcard-example-com"
	if *actual != expected {
		t.Errorf("Expected %s, got %s", expected, *actual)
	}
}
