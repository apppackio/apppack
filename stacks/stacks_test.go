package stacks

import (
	"testing"

	"github.com/apppackio/apppack/ui/uitest"
	"github.com/charmbracelet/huh"
)

func TestClusterSelectForm_SelectFirst(t *testing.T) {
	options := []huh.Option[string]{
		huh.NewOption("production", "apppack-cluster-production"),
		huh.NewOption("staging", "apppack-cluster-staging"),
	}

	form, selectedPtr := ClusterSelectForm(options, "Pick a cluster", "")
	tm := uitest.RunForm(t, form)
	// Pass the Note, then accept first Select option
	uitest.SelectFirst(tm)
	uitest.SelectFirst(tm)
	uitest.WaitDone(t, tm)

	if *selectedPtr != "apppack-cluster-production" {
		t.Errorf("expected 'apppack-cluster-production', got %q", *selectedPtr)
	}
}

func TestClusterSelectForm_SelectSecond(t *testing.T) {
	options := []huh.Option[string]{
		huh.NewOption("production", "apppack-cluster-production"),
		huh.NewOption("staging", "apppack-cluster-staging"),
	}

	form, selectedPtr := ClusterSelectForm(options, "Pick a cluster", "")
	tm := uitest.RunForm(t, form)
	// Pass the Note, then select second option
	uitest.SelectFirst(tm)
	uitest.SelectNth(tm, 1)
	uitest.WaitDone(t, tm)

	if *selectedPtr != "apppack-cluster-staging" {
		t.Errorf("expected 'apppack-cluster-staging', got %q", *selectedPtr)
	}
}
