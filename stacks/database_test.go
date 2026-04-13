package stacks

import (
	"testing"

	"github.com/apppackio/apppack/ui/uitest"
)

func TestDatabaseEngineForm_DefaultPostgres(t *testing.T) {
	form, selectedPtr := DatabaseEngineForm("postgres")
	tm := uitest.RunForm(t, form)
	uitest.SelectFirst(tm) // pass Note
	uitest.SelectFirst(tm) // accept default (postgres)
	uitest.WaitDone(t, tm)

	if *selectedPtr != "postgres" {
		t.Errorf("expected 'postgres', got %q", *selectedPtr)
	}
}

func TestDatabaseEngineForm_SelectMySQL(t *testing.T) {
	form, selectedPtr := DatabaseEngineForm("postgres")
	tm := uitest.RunForm(t, form)
	uitest.SelectFirst(tm)  // pass Note
	uitest.SelectNth(tm, 1) // select mysql
	uitest.WaitDone(t, tm)

	if *selectedPtr != "mysql" {
		t.Errorf("expected 'mysql', got %q", *selectedPtr)
	}
}

func TestDatabaseAuroraForm_DefaultNo(t *testing.T) {
	form, selectedPtr := DatabaseAuroraForm(false)
	tm := uitest.RunForm(t, form)
	uitest.SelectFirst(tm) // pass Note
	uitest.SelectFirst(tm) // accept default (no)
	uitest.WaitDone(t, tm)

	if *selectedPtr != "no" {
		t.Errorf("expected 'no', got %q", *selectedPtr)
	}
}

func TestDatabaseAuroraForm_DefaultYes(t *testing.T) {
	form, selectedPtr := DatabaseAuroraForm(true)
	tm := uitest.RunForm(t, form)
	uitest.SelectFirst(tm) // pass Note
	uitest.SelectFirst(tm) // accept default (yes)
	uitest.WaitDone(t, tm)

	if *selectedPtr != "yes" {
		t.Errorf("expected 'yes', got %q", *selectedPtr)
	}
}

func TestDatabaseInstanceClassForm_SelectDefault(t *testing.T) {
	classes := []string{"db.t4g.medium", "db.t4g.large", "db.r6g.large"}

	form, selectedPtr := DatabaseInstanceClassForm(classes, "db.t4g.medium")
	tm := uitest.RunForm(t, form)
	uitest.SelectFirst(tm) // pass Note
	uitest.SelectFirst(tm) // accept default
	uitest.WaitDone(t, tm)

	if *selectedPtr != "db.t4g.medium" {
		t.Errorf("expected 'db.t4g.medium', got %q", *selectedPtr)
	}
}

func TestDatabaseInstanceClassForm_SelectSecond(t *testing.T) {
	classes := []string{"db.t4g.medium", "db.t4g.large", "db.r6g.large"}

	form, selectedPtr := DatabaseInstanceClassForm(classes, "db.t4g.medium")
	tm := uitest.RunForm(t, form)
	uitest.SelectFirst(tm)  // pass Note
	uitest.SelectNth(tm, 1) // select second option
	uitest.WaitDone(t, tm)

	if *selectedPtr != "db.t4g.large" {
		t.Errorf("expected 'db.t4g.large', got %q", *selectedPtr)
	}
}

func TestDatabaseMultiAZForm_DefaultNo(t *testing.T) {
	form, selectedPtr := DatabaseMultiAZForm(false)
	tm := uitest.RunForm(t, form)
	uitest.SelectFirst(tm) // pass Note
	uitest.SelectFirst(tm) // accept default (no)
	uitest.WaitDone(t, tm)

	if *selectedPtr != "no" {
		t.Errorf("expected 'no', got %q", *selectedPtr)
	}
}

func TestDatabaseMultiAZForm_DefaultYes(t *testing.T) {
	form, selectedPtr := DatabaseMultiAZForm(true)
	tm := uitest.RunForm(t, form)
	uitest.SelectFirst(tm) // pass Note
	uitest.SelectFirst(tm) // accept default (yes)
	uitest.WaitDone(t, tm)

	if *selectedPtr != "yes" {
		t.Errorf("expected 'yes', got %q", *selectedPtr)
	}
}
