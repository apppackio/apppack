package stacks

import (
	"testing"

	"github.com/apppackio/apppack/ui/uitest"
	"github.com/spf13/pflag"
)

func TestValidateDatabaseEngine(t *testing.T) {
	valid := []string{"mysql", "postgres", "aurora-mysql", "aurora-postgresql"}
	for _, e := range valid {
		if err := validateDatabaseEngine(e); err != nil {
			t.Errorf("expected %q to be valid, got error: %v", e, err)
		}
	}

	invalid := []string{"", "sqlite", "aurora", "Postgres", "mariadb"}
	for _, e := range invalid {
		if err := validateDatabaseEngine(e); err == nil {
			t.Errorf("expected %q to be invalid, got no error", e)
		}
	}
}

// newDatabaseFlagSet mirrors the flags registered on createDatabaseCmd so we can
// exercise UpdateFromFlags without pulling in the cmd package.
func newDatabaseFlagSet() *pflag.FlagSet {
	flags := pflag.NewFlagSet("database", pflag.ContinueOnError)
	flags.String("cluster", "apppack", "cluster name")
	flags.StringP("instance-class", "i", DefaultDatabaseStackParameters.InstanceClass, "instance class")
	flags.StringP("engine", "e", DefaultDatabaseStackParameters.Engine, "engine")
	flags.Bool("multi-az", DefaultDatabaseStackParameters.MultiAZ, "enable multi-AZ")
	flags.Int("allocated-storage", DefaultDatabaseStackParameters.AllocatedStorage, "allocated storage")
	flags.Int("max-allocated-storage", DefaultDatabaseStackParameters.MaxAllocatedStorage, "max allocated storage")

	return flags
}

func TestUpdateFromFlagsRecordsSetFlags(t *testing.T) {
	flags := newDatabaseFlagSet()
	if err := flags.Parse([]string{"--engine", "aurora-postgresql", "--multi-az"}); err != nil {
		t.Fatalf("failed to parse flags: %v", err)
	}

	params := DefaultDatabaseStackParameters
	stack := DatabaseStack{Parameters: &params}
	if err := stack.UpdateFromFlags(flags); err != nil {
		t.Fatalf("UpdateFromFlags returned error: %v", err)
	}

	if !stack.flagWasSet("engine") {
		t.Error("expected engine flag to be recorded as set")
	}
	if !stack.flagWasSet("multi-az") {
		t.Error("expected multi-az flag to be recorded as set")
	}
	if stack.flagWasSet("cluster") {
		t.Error("expected cluster flag to be recorded as not set")
	}
	if stack.flagWasSet("instance-class") {
		t.Error("expected instance-class flag to be recorded as not set")
	}

	if params.Engine != "aurora-postgresql" {
		t.Errorf("expected engine 'aurora-postgresql', got %q", params.Engine)
	}
	if !params.MultiAZ {
		t.Error("expected multi-az to be true")
	}
}

func TestUpdateFromFlagsNoFlagsSet(t *testing.T) {
	flags := newDatabaseFlagSet()
	if err := flags.Parse([]string{}); err != nil {
		t.Fatalf("failed to parse flags: %v", err)
	}

	params := DefaultDatabaseStackParameters
	stack := DatabaseStack{Parameters: &params}
	if err := stack.UpdateFromFlags(flags); err != nil {
		t.Fatalf("UpdateFromFlags returned error: %v", err)
	}

	for _, name := range []string{"cluster", "engine", "instance-class", "multi-az"} {
		if stack.flagWasSet(name) {
			t.Errorf("expected %q to be recorded as not set", name)
		}
	}
}

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
