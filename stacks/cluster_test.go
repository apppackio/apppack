package stacks

import (
	"testing"

	"github.com/apppackio/apppack/ui/uitest"
)

func TestClusterDomainForm_EnterDomain(t *testing.T) {
	form, domainPtr := ClusterDomainForm("")
	tm := uitest.RunForm(t, form)
	uitest.TypeAndSubmit(tm, "example.com")
	uitest.WaitDone(t, tm)

	if *domainPtr != "example.com" {
		t.Errorf("expected 'example.com', got %q", *domainPtr)
	}
}

func TestClusterDomainForm_DefaultDomain(t *testing.T) {
	form, domainPtr := ClusterDomainForm("default.io")
	tm := uitest.RunForm(t, form)
	// Press Enter twice: once to pass the Note, once to accept the default Input
	uitest.SelectFirst(tm)
	uitest.SelectFirst(tm)
	uitest.WaitDone(t, tm)

	if *domainPtr != "default.io" {
		t.Errorf("expected 'default.io', got %q", *domainPtr)
	}
}
