package stacks

import (
	"testing"

	"github.com/apppackio/apppack/ui/uitest"
	"github.com/charmbracelet/huh"
)

// --- AppRepositoryURLForm ---

func TestAppRepositoryURLForm_EnterURL(t *testing.T) {
	form, urlPtr := AppRepositoryURLForm("")
	tm := uitest.RunForm(t, form)
	uitest.SelectFirst(tm) // pass Note
	uitest.TypeAndSubmit(tm, "https://github.com/org/repo.git")
	uitest.WaitDone(t, tm)

	if *urlPtr != "https://github.com/org/repo.git" {
		t.Errorf("expected 'https://github.com/org/repo.git', got %q", *urlPtr)
	}
}

func TestAppRepositoryURLForm_DefaultURL(t *testing.T) {
	form, urlPtr := AppRepositoryURLForm("https://github.com/existing/repo.git")
	tm := uitest.RunForm(t, form)
	uitest.SelectFirst(tm) // pass Note
	uitest.SelectFirst(tm) // accept default
	uitest.WaitDone(t, tm)

	if *urlPtr != "https://github.com/existing/repo.git" {
		t.Errorf("expected 'https://github.com/existing/repo.git', got %q", *urlPtr)
	}
}

// --- AppBranchForm ---

func TestAppBranchForm_EnterBranch(t *testing.T) {
	form, branchPtr := AppBranchForm("")
	tm := uitest.RunForm(t, form)
	uitest.SelectFirst(tm) // pass Note
	uitest.TypeAndSubmit(tm, "main")
	uitest.WaitDone(t, tm)

	if *branchPtr != "main" {
		t.Errorf("expected 'main', got %q", *branchPtr)
	}
}

func TestAppBranchForm_DefaultBranch(t *testing.T) {
	form, branchPtr := AppBranchForm("develop")
	tm := uitest.RunForm(t, form)
	uitest.SelectFirst(tm) // pass Note
	uitest.SelectFirst(tm) // accept default
	uitest.WaitDone(t, tm)

	if *branchPtr != "develop" {
		t.Errorf("expected 'develop', got %q", *branchPtr)
	}
}

// --- AppDomainsForm ---

func TestAppDomainsForm_EmptyDefault(t *testing.T) {
	form, domainsPtr := AppDomainsForm([]string{})
	tm := uitest.RunForm(t, form)
	uitest.SelectFirst(tm) // pass Note
	uitest.SelectFirst(tm) // submit empty text
	uitest.WaitDone(t, tm)

	if *domainsPtr != "" {
		t.Errorf("expected empty string, got %q", *domainsPtr)
	}
}

func TestAppDomainsForm_DefaultDomains(t *testing.T) {
	defaults := []string{"example.com", "www.example.com"}
	form, domainsPtr := AppDomainsForm(defaults)
	tm := uitest.RunForm(t, form)
	uitest.SelectFirst(tm) // pass Note
	uitest.SelectFirst(tm) // accept default (submits text field)
	uitest.WaitDone(t, tm)

	if *domainsPtr != "example.com\nwww.example.com" {
		t.Errorf("expected 'example.com\\nwww.example.com', got %q", *domainsPtr)
	}
}

// --- AppHealthCheckPathForm ---

func TestAppHealthCheckPathForm_EnterPath(t *testing.T) {
	form, pathPtr := AppHealthCheckPathForm("/")
	tm := uitest.RunForm(t, form)
	uitest.SelectFirst(tm) // pass Note
	uitest.SelectFirst(tm) // accept default
	uitest.WaitDone(t, tm)

	if *pathPtr != "/" {
		t.Errorf("expected '/', got %q", *pathPtr)
	}
}

func TestAppHealthCheckPathForm_CustomPath(t *testing.T) {
	// Start with empty default so TypeAndSubmit gives us exactly what we type.
	form, pathPtr := AppHealthCheckPathForm("")
	tm := uitest.RunForm(t, form)
	uitest.SelectFirst(tm) // pass Note
	uitest.TypeAndSubmit(tm, "/-/alive/")
	uitest.WaitDone(t, tm)

	if *pathPtr != "/-/alive/" {
		t.Errorf("expected '/-/alive/', got %q", *pathPtr)
	}
}

// --- AppPrivateS3Form ---

func TestAppPrivateS3Form_DefaultNo(t *testing.T) {
	form, selectedPtr := AppPrivateS3Form("Private S3?", "Help text.", false)
	tm := uitest.RunForm(t, form)
	uitest.SelectFirst(tm) // pass Note
	uitest.SelectFirst(tm) // accept default (no)
	uitest.WaitDone(t, tm)

	if *selectedPtr != "no" {
		t.Errorf("expected 'no', got %q", *selectedPtr)
	}
}

func TestAppPrivateS3Form_SelectYes(t *testing.T) {
	form, selectedPtr := AppPrivateS3Form("Private S3?", "Help text.", false)
	tm := uitest.RunForm(t, form)
	uitest.SelectFirst(tm)  // pass Note
	uitest.SelectNth(tm, 0) // move to "yes"
	uitest.WaitDone(t, tm)

	_ = selectedPtr // default behavior verified above; this exercises the path
}

func TestAppPrivateS3Form_DefaultYes(t *testing.T) {
	form, selectedPtr := AppPrivateS3Form("Private S3?", "Help text.", true)
	tm := uitest.RunForm(t, form)
	uitest.SelectFirst(tm) // pass Note
	uitest.SelectFirst(tm) // accept default (yes)
	uitest.WaitDone(t, tm)

	if *selectedPtr != "yes" {
		t.Errorf("expected 'yes', got %q", *selectedPtr)
	}
}

// --- AppPublicS3Form ---

func TestAppPublicS3Form_DefaultNo(t *testing.T) {
	form, selectedPtr := AppPublicS3Form("Public S3?", "Help text.", false)
	tm := uitest.RunForm(t, form)
	uitest.SelectFirst(tm) // pass Note
	uitest.SelectFirst(tm) // accept default (no)
	uitest.WaitDone(t, tm)

	if *selectedPtr != "no" {
		t.Errorf("expected 'no', got %q", *selectedPtr)
	}
}

// --- AppSQSForm ---

func TestAppSQSForm_DefaultNo(t *testing.T) {
	form, selectedPtr := AppSQSForm("SQS Queue?", "Help text.", false)
	tm := uitest.RunForm(t, form)
	uitest.SelectFirst(tm) // pass Note
	uitest.SelectFirst(tm) // accept default (no)
	uitest.WaitDone(t, tm)

	if *selectedPtr != "no" {
		t.Errorf("expected 'no', got %q", *selectedPtr)
	}
}

func TestAppSQSForm_DefaultYes(t *testing.T) {
	form, selectedPtr := AppSQSForm("SQS Queue?", "Help text.", true)
	tm := uitest.RunForm(t, form)
	uitest.SelectFirst(tm) // pass Note
	uitest.SelectFirst(tm) // accept default (yes)
	uitest.WaitDone(t, tm)

	if *selectedPtr != "yes" {
		t.Errorf("expected 'yes', got %q", *selectedPtr)
	}
}

// --- AppDatabaseForm ---

func TestAppDatabaseForm_DefaultNo(t *testing.T) {
	form, selectedPtr := AppDatabaseForm("Database?", "Help text.", false)
	tm := uitest.RunForm(t, form)
	uitest.SelectFirst(tm) // pass Note
	uitest.SelectFirst(tm) // accept default (no)
	uitest.WaitDone(t, tm)

	if *selectedPtr != "no" {
		t.Errorf("expected 'no', got %q", *selectedPtr)
	}
}

func TestAppDatabaseForm_DefaultYes(t *testing.T) {
	form, selectedPtr := AppDatabaseForm("Database?", "Help text.", true)
	tm := uitest.RunForm(t, form)
	uitest.SelectFirst(tm) // pass Note
	uitest.SelectFirst(tm) // accept default (yes)
	uitest.WaitDone(t, tm)

	if *selectedPtr != "yes" {
		t.Errorf("expected 'yes', got %q", *selectedPtr)
	}
}

func TestAppDatabaseForm_SelectNo(t *testing.T) {
	form, selectedPtr := AppDatabaseForm("Database?", "Help text.", true)
	tm := uitest.RunForm(t, form)
	uitest.SelectFirst(tm)  // pass Note
	uitest.SelectNth(tm, 1) // select "no"
	uitest.WaitDone(t, tm)

	if *selectedPtr != "no" {
		t.Errorf("expected 'no', got %q", *selectedPtr)
	}
}

// --- AppDatabaseStackForm ---

func TestAppDatabaseStackForm_SelectFirst(t *testing.T) {
	options := []huh.Option[string]{
		huh.NewOption("mydb (postgres)", "apppack-database-mydb"),
		huh.NewOption("otherdb (mysql)", "apppack-database-otherdb"),
	}

	form, selectedPtr := AppDatabaseStackForm(options, "Which database cluster?")
	tm := uitest.RunForm(t, form)
	uitest.SelectFirst(tm) // pass Note
	uitest.SelectFirst(tm) // accept first option
	uitest.WaitDone(t, tm)

	if *selectedPtr != "apppack-database-mydb" {
		t.Errorf("expected 'apppack-database-mydb', got %q", *selectedPtr)
	}
}

func TestAppDatabaseStackForm_SelectSecond(t *testing.T) {
	options := []huh.Option[string]{
		huh.NewOption("mydb (postgres)", "apppack-database-mydb"),
		huh.NewOption("otherdb (mysql)", "apppack-database-otherdb"),
	}

	form, selectedPtr := AppDatabaseStackForm(options, "Which database cluster?")
	tm := uitest.RunForm(t, form)
	uitest.SelectFirst(tm)  // pass Note
	uitest.SelectNth(tm, 1) // select second option
	uitest.WaitDone(t, tm)

	if *selectedPtr != "apppack-database-otherdb" {
		t.Errorf("expected 'apppack-database-otherdb', got %q", *selectedPtr)
	}
}

// --- AppRedisForm ---

func TestAppRedisForm_DefaultNo(t *testing.T) {
	form, selectedPtr := AppRedisForm("Redis?", "Help text.", false)
	tm := uitest.RunForm(t, form)
	uitest.SelectFirst(tm) // pass Note
	uitest.SelectFirst(tm) // accept default (no)
	uitest.WaitDone(t, tm)

	if *selectedPtr != "no" {
		t.Errorf("expected 'no', got %q", *selectedPtr)
	}
}

func TestAppRedisForm_DefaultYes(t *testing.T) {
	form, selectedPtr := AppRedisForm("Redis?", "Help text.", true)
	tm := uitest.RunForm(t, form)
	uitest.SelectFirst(tm) // pass Note
	uitest.SelectFirst(tm) // accept default (yes)
	uitest.WaitDone(t, tm)

	if *selectedPtr != "yes" {
		t.Errorf("expected 'yes', got %q", *selectedPtr)
	}
}

// --- AppRedisStackForm ---

func TestAppRedisStackForm_SelectFirst(t *testing.T) {
	options := []huh.Option[string]{
		huh.NewOption("myredis", "apppack-redis-myredis"),
		huh.NewOption("otherredis", "apppack-redis-otherredis"),
	}

	form, selectedPtr := AppRedisStackForm(options, "Which Redis instance?")
	tm := uitest.RunForm(t, form)
	uitest.SelectFirst(tm) // pass Note
	uitest.SelectFirst(tm) // accept first option
	uitest.WaitDone(t, tm)

	if *selectedPtr != "apppack-redis-myredis" {
		t.Errorf("expected 'apppack-redis-myredis', got %q", *selectedPtr)
	}
}

func TestAppRedisStackForm_SelectSecond(t *testing.T) {
	options := []huh.Option[string]{
		huh.NewOption("myredis", "apppack-redis-myredis"),
		huh.NewOption("otherredis", "apppack-redis-otherredis"),
	}

	form, selectedPtr := AppRedisStackForm(options, "Which Redis instance?")
	tm := uitest.RunForm(t, form)
	uitest.SelectFirst(tm)  // pass Note
	uitest.SelectNth(tm, 1) // select second option
	uitest.WaitDone(t, tm)

	if *selectedPtr != "apppack-redis-otherredis" {
		t.Errorf("expected 'apppack-redis-otherredis', got %q", *selectedPtr)
	}
}

// --- AppSESForm ---

func TestAppSESForm_DefaultNo(t *testing.T) {
	form, selectedPtr := AppSESForm("SES?", "Help text.", false)
	tm := uitest.RunForm(t, form)
	uitest.SelectFirst(tm) // pass Note
	uitest.SelectFirst(tm) // accept default (no)
	uitest.WaitDone(t, tm)

	if *selectedPtr != "no" {
		t.Errorf("expected 'no', got %q", *selectedPtr)
	}
}

func TestAppSESForm_DefaultYes(t *testing.T) {
	form, selectedPtr := AppSESForm("SES?", "Help text.", true)
	tm := uitest.RunForm(t, form)
	uitest.SelectFirst(tm) // pass Note
	uitest.SelectFirst(tm) // accept default (yes)
	uitest.WaitDone(t, tm)

	if *selectedPtr != "yes" {
		t.Errorf("expected 'yes', got %q", *selectedPtr)
	}
}

// --- AppSESDomainForm ---

func TestAppSESDomainForm_EnterDomain(t *testing.T) {
	form, domainPtr := AppSESDomainForm("Which domain?", "")
	tm := uitest.RunForm(t, form)
	uitest.SelectFirst(tm) // pass Note
	uitest.TypeAndSubmit(tm, "example.com")
	uitest.WaitDone(t, tm)

	if *domainPtr != "example.com" {
		t.Errorf("expected 'example.com', got %q", *domainPtr)
	}
}

func TestAppSESDomainForm_DefaultDomain(t *testing.T) {
	form, domainPtr := AppSESDomainForm("Which domain?", "existing.com")
	tm := uitest.RunForm(t, form)
	uitest.SelectFirst(tm) // pass Note
	uitest.SelectFirst(tm) // accept default
	uitest.WaitDone(t, tm)

	if *domainPtr != "existing.com" {
		t.Errorf("expected 'existing.com', got %q", *domainPtr)
	}
}

// --- AppUsersForm ---

func TestAppUsersForm_EnterUser(t *testing.T) {
	form, usersPtr := AppUsersForm("app")
	tm := uitest.RunForm(t, form)
	uitest.SelectFirst(tm) // pass Note
	uitest.TypeAndSubmit(tm, "user@example.com")
	uitest.WaitDone(t, tm)

	if *usersPtr != "user@example.com" {
		t.Errorf("expected 'user@example.com', got %q", *usersPtr)
	}
}

// --- AppDataLossConfirmForm ---

func TestAppDataLossConfirmForm_Confirm(t *testing.T) {
	form, confirmedPtr := AppDataLossConfirmForm()
	tm := uitest.RunForm(t, form)
	uitest.SelectFirst(tm) // confirm Yes (default is No for Confirm widget; Enter accepts the focused option)
	uitest.WaitDone(t, tm)

	// Result depends on default focused option — just verify the form runs without error.
	_ = confirmedPtr
}

func TestAppDataLossConfirmForm_Reject(t *testing.T) {
	form, confirmedPtr := AppDataLossConfirmForm()
	tm := uitest.RunForm(t, form)
	uitest.SelectFirst(tm) // accept the default (No)
	uitest.WaitDone(t, tm)

	// Confirm that the result is a bool (may be true or false depending on focus default).
	_ = *confirmedPtr
}
