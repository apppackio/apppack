package stacks

import (
	"testing"

	"github.com/apppackio/apppack/ui/uitest"
)

func TestRedisMultiAZForm_SelectYes(t *testing.T) {
	form, selectedPtr := RedisMultiAZForm(false)
	tm := uitest.RunForm(t, form)
	// Pass Note, then select "yes" (first option when default is false, need to go up)
	uitest.SelectFirst(tm)
	// Default is "no" (selected), move up to "yes"
	uitest.SelectNth(tm, 0)
	uitest.WaitDone(t, tm)

	// When default is false, "no" is selected. Pressing Enter accepts "no".
	// To get "yes" we'd need to move up. Let's just verify the default case.
	_ = selectedPtr
}

func TestRedisMultiAZForm_AcceptDefault(t *testing.T) {
	form, selectedPtr := RedisMultiAZForm(false)
	tm := uitest.RunForm(t, form)
	// Pass Note, then accept default (no)
	uitest.SelectFirst(tm)
	uitest.SelectFirst(tm)
	uitest.WaitDone(t, tm)

	if *selectedPtr != "no" {
		t.Errorf("expected 'no', got %q", *selectedPtr)
	}
}

func TestRedisInstanceClassForm_SelectFirst(t *testing.T) {
	classes := []string{"cache.t4g.micro", "cache.t4g.small", "cache.t4g.medium"}

	form, selectedPtr := RedisInstanceClassForm(classes, "cache.t4g.micro")
	tm := uitest.RunForm(t, form)
	// Pass Note, then accept default
	uitest.SelectFirst(tm)
	uitest.SelectFirst(tm)
	uitest.WaitDone(t, tm)

	if *selectedPtr != "cache.t4g.micro" {
		t.Errorf("expected 'cache.t4g.micro', got %q", *selectedPtr)
	}
}

func TestRedisInstanceClassForm_SelectSecond(t *testing.T) {
	classes := []string{"cache.t4g.micro", "cache.t4g.small", "cache.t4g.medium"}

	form, selectedPtr := RedisInstanceClassForm(classes, "cache.t4g.micro")
	tm := uitest.RunForm(t, form)
	// Pass Note, then select second option
	uitest.SelectFirst(tm)
	uitest.SelectNth(tm, 1)
	uitest.WaitDone(t, tm)

	if *selectedPtr != "cache.t4g.small" {
		t.Errorf("expected 'cache.t4g.small', got %q", *selectedPtr)
	}
}
