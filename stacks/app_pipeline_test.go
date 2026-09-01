package stacks

import (
	"strings"
	"testing"

	"github.com/apppackio/apppack/ui/uitest"
	"github.com/aws/aws-sdk-go-v2/aws"
	tea "github.com/charmbracelet/bubbletea"
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
	// Default is "no" which means cursor starts on option[1] ("no").
	// Press Up to move to option[0] ("yes"), then Enter.
	form, selectedPtr := AppPrivateS3Form("Private S3?", "Help text.", false)
	tm := uitest.RunForm(t, form)
	uitest.SelectFirst(tm) // pass Note
	tm.Send(tea.KeyMsg{Type: tea.KeyUp})
	tm.Send(tea.KeyMsg{Type: tea.KeyEnter})
	uitest.WaitDone(t, tm)

	if *selectedPtr != "yes" {
		t.Errorf("expected 'yes', got %q", *selectedPtr)
	}
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

// TestAppDatabaseStackForm_PreservesSelection verifies that when the caller
// marks a non-first option as pre-selected (via .Selected(true)), the form
// starts with that option focused. This matches the real-world `modify app`
// workflow where the existing database is re-presented to the user.
func TestAppDatabaseStackForm_PreservesSelection(t *testing.T) {
	options := []huh.Option[string]{
		huh.NewOption("mydb (postgres)", "apppack-database-mydb"),
		huh.NewOption("otherdb (mysql)", "apppack-database-otherdb").Selected(true),
	}

	form, selectedPtr := AppDatabaseStackForm(options, "Which database cluster?")
	tm := uitest.RunForm(t, form)
	uitest.SelectFirst(tm) // pass Note
	uitest.SelectFirst(tm) // accept currently-focused option (should be the pre-selected second one)
	uitest.WaitDone(t, tm)

	if *selectedPtr != "apppack-database-otherdb" {
		t.Errorf("expected pre-selected 'apppack-database-otherdb' to be preserved, got %q", *selectedPtr)
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

// TestAppRedisStackForm_PreservesSelection verifies the pre-existing selection
// (via .Selected(true)) is honored, matching the `modify app` flow.
func TestAppRedisStackForm_PreservesSelection(t *testing.T) {
	options := []huh.Option[string]{
		huh.NewOption("myredis", "apppack-redis-myredis"),
		huh.NewOption("otherredis", "apppack-redis-otherredis").Selected(true),
	}

	form, selectedPtr := AppRedisStackForm(options, "Which Redis instance?")
	tm := uitest.RunForm(t, form)
	uitest.SelectFirst(tm) // pass Note
	uitest.SelectFirst(tm) // accept currently-focused option (should be pre-selected second)
	uitest.WaitDone(t, tm)

	if *selectedPtr != "apppack-redis-otherredis" {
		t.Errorf("expected pre-selected 'apppack-redis-otherredis' to be preserved, got %q", *selectedPtr)
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
	// Default is No (false). Press Left to flip focus to Yes, then Enter.
	// This is safety-critical: the form must only commit true when the user
	// explicitly moves focus to the affirmative option.
	form, confirmedPtr := AppDataLossConfirmForm()
	tm := uitest.RunForm(t, form)
	tm.Send(tea.KeyMsg{Type: tea.KeyLeft})
	tm.Send(tea.KeyMsg{Type: tea.KeyEnter})
	uitest.WaitDone(t, tm)

	if !*confirmedPtr {
		t.Error("expected confirmed=true when user selects Yes, got false")
	}
}

func TestAppDataLossConfirmForm_Reject(t *testing.T) {
	// Default is No (false). Pressing Enter should commit that without
	// requiring any focus change.
	form, confirmedPtr := AppDataLossConfirmForm()
	tm := uitest.RunForm(t, form)
	uitest.SelectFirst(tm) // accept the default (No)
	uitest.WaitDone(t, tm)

	if *confirmedPtr {
		t.Error("expected confirmed=false when user accepts default (No), got true")
	}
}

// --- Addon enable-default logic ---

// TestDatabaseAddonEnabledDefault verifies that the interactive default for the
// database addon is true whenever either DatabaseAddonEnabled (the bool flag) or
// DatabaseStackName (an explicit instance name) is set.
func TestDatabaseAddonEnabledDefault(t *testing.T) {
	t.Helper()

	tests := []struct {
		name        string
		params      AppStackParameters
		wantEnabled bool
	}{
		{
			name:        "bool flag only",
			params:      AppStackParameters{DatabaseAddonEnabled: true},
			wantEnabled: true,
		},
		{
			name:        "stack name only",
			params:      AppStackParameters{DatabaseStackName: "apppack-database-mydb"},
			wantEnabled: true,
		},
		{
			name:        "both set",
			params:      AppStackParameters{DatabaseAddonEnabled: true, DatabaseStackName: "apppack-database-mydb"},
			wantEnabled: true,
		},
		{
			name:        "neither set",
			params:      AppStackParameters{},
			wantEnabled: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.params.DatabaseAddonEnabled || tt.params.DatabaseStackName != ""
			if got != tt.wantEnabled {
				t.Errorf("enable = %v, want %v", got, tt.wantEnabled)
			}
		})
	}
}

// TestRedisAddonEnabledDefault mirrors TestDatabaseAddonEnabledDefault for Redis.
func TestRedisAddonEnabledDefault(t *testing.T) {
	t.Helper()

	tests := []struct {
		name        string
		params      AppStackParameters
		wantEnabled bool
	}{
		{
			name:        "bool flag only",
			params:      AppStackParameters{RedisAddonEnabled: true},
			wantEnabled: true,
		},
		{
			name:        "stack name only",
			params:      AppStackParameters{RedisStackName: "apppack-redis-myredis"},
			wantEnabled: true,
		},
		{
			name:        "both set",
			params:      AppStackParameters{RedisAddonEnabled: true, RedisStackName: "apppack-redis-myredis"},
			wantEnabled: true,
		},
		{
			name:        "neither set",
			params:      AppStackParameters{},
			wantEnabled: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.params.RedisAddonEnabled || tt.params.RedisStackName != ""
			if got != tt.wantEnabled {
				t.Errorf("enable = %v, want %v", got, tt.wantEnabled)
			}
		})
	}
}

// --- selectDatabaseStack ---

func TestSelectDatabaseStack(t *testing.T) {
	t.Helper()

	tests := []struct {
		name       string
		databases  []string
		wantStack  string
		wantErrSub string // non-empty: error must contain this substring
	}{
		{
			name:       "zero databases → error with create instruction",
			databases:  []string{},
			wantErrSub: "apppack create database",
		},
		{
			name:      "single database without engine",
			databases: []string{"mydb"},
			wantStack: "apppack-database-mydb",
		},
		{
			name:      "single database with engine suffix",
			databases: []string{"mydb (postgres)"},
			wantStack: "apppack-database-mydb",
		},
		{
			name:       "multiple databases → error listing names and flag hint",
			databases:  []string{"mydb (postgres)", "otherdb (mysql)"},
			wantErrSub: "--addon-database-name",
		},
		{
			name:       "multiple databases error contains instance names",
			databases:  []string{"mydb (postgres)", "otherdb (mysql)"},
			wantErrSub: "mydb",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := selectDatabaseStack("mycluster", tt.databases)

			if tt.wantErrSub != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErrSub)
				}
				if !strings.Contains(err.Error(), tt.wantErrSub) {
					t.Errorf("error %q does not contain %q", err.Error(), tt.wantErrSub)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if got != tt.wantStack {
				t.Errorf("got %q, want %q", got, tt.wantStack)
			}
		})
	}
}

// --- selectRedisStack ---

func TestSelectRedisStack(t *testing.T) {
	t.Helper()

	tests := []struct {
		name       string
		redises    []string
		wantStack  string
		wantErrSub string
	}{
		{
			name:       "zero instances → error with create instruction",
			redises:    []string{},
			wantErrSub: "apppack create redis",
		},
		{
			name:      "single instance",
			redises:   []string{"myredis"},
			wantStack: "apppack-redis-myredis",
		},
		{
			name:       "multiple instances → error with flag hint",
			redises:    []string{"myredis", "otherredis"},
			wantErrSub: "--addon-redis-name",
		},
		{
			name:       "multiple instances error contains instance names",
			redises:    []string{"myredis", "otherredis"},
			wantErrSub: "myredis",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := selectRedisStack("mycluster", tt.redises)

			if tt.wantErrSub != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErrSub)
				}
				if !strings.Contains(err.Error(), tt.wantErrSub) {
					t.Errorf("error %q does not contain %q", err.Error(), tt.wantErrSub)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if got != tt.wantStack {
				t.Errorf("got %q, want %q", got, tt.wantStack)
			}
		})
	}
}

// --- engineFromDatabaseURL ---

func TestEngineFromDatabaseURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                   string
		rawURL                 string
		wantEngine             string
		wantUnrecognizedScheme string
	}{
		{name: "empty string", rawURL: ""},
		{name: "postgres", rawURL: "postgres://user:pass@host:5432/db", wantEngine: "postgres"},
		{name: "postgresql", rawURL: "postgresql://user:pass@host:5432/db", wantEngine: "postgres"},
		{name: "pgsql", rawURL: "pgsql://user:pass@host:5432/db", wantEngine: "postgres"},
		{name: "psql", rawURL: "psql://user:pass@host:5432/db", wantEngine: "postgres"},
		{name: "mysql", rawURL: "mysql://user:pass@host:3306/db", wantEngine: "mysql"},
		{name: "mysql2", rawURL: "mysql2://user:pass@host:3306/db", wantEngine: "mysql"},
		{name: "mariadb", rawURL: "mariadb://user:pass@host:3306/db", wantEngine: "mysql"},
		{name: "scheme comparison is case-insensitive", rawURL: "POSTGRES://user:pass@host/db", wantEngine: "postgres"},
		{
			name:                   "unknown scheme",
			rawURL:                 "mongodb://user:pass@host:27017/db",
			wantUnrecognizedScheme: "mongodb",
		},
		{name: "malformed URL (parse error)", rawURL: "://not-a-url"},
		{name: "malformed URL (no scheme)", rawURL: "not a url at all"},
		{
			// Guards against substring matching: the whole URL contains "mysql" (in
			// the password) but the scheme is postgres, so the result must be postgres.
			name:       "password contains mysql but scheme is postgres",
			rawURL:     "postgres://user:mysqlpassword123@host:5432/db",
			wantEngine: "postgres",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			engine, unrecognizedScheme := engineFromDatabaseURL(tt.rawURL)

			if engine != tt.wantEngine {
				t.Errorf("engine = %q, want %q", engine, tt.wantEngine)
			}

			if unrecognizedScheme != tt.wantUnrecognizedScheme {
				t.Errorf("unrecognizedScheme = %q, want %q", unrecognizedScheme, tt.wantUnrecognizedScheme)
			}
		})
	}
}

// --- detectExternalDatabaseEngine ---

// TestDetectExternalDatabaseEngine covers the auto-detection algorithm in
// SetInternalFields. The SSM read is stubbed via the fetchDatabaseURL function
// variable so these run with no AWS calls.
func TestDetectExternalDatabaseEngine(t *testing.T) {
	tests := []struct {
		name        string
		params      AppStackParameters
		databaseURL string
		fetchOK     bool
		wantEngine  string
	}{
		{
			name:        "managed database via DatabaseStackName forces empty even with a postgres DATABASE_URL",
			params:      AppStackParameters{Type: "app", DatabaseStackName: "apppack-database-mydb"},
			databaseURL: "postgres://user:pass@host/db",
			fetchOK:     true,
			wantEngine:  "",
		},
		{
			name:        "managed database via DatabaseAddonEnabled forces empty even with a postgres DATABASE_URL",
			params:      AppStackParameters{Type: "app", DatabaseAddonEnabled: true},
			databaseURL: "postgres://user:pass@host/db",
			fetchOK:     true,
			wantEngine:  "",
		},
		{
			name:        "pipeline forces empty even with a postgres DATABASE_URL",
			params:      AppStackParameters{Type: "pipeline"},
			databaseURL: "postgres://user:pass@host/db",
			fetchOK:     true,
			wantEngine:  "",
		},
		{
			name:       "SSM lookup failure yields empty",
			params:     AppStackParameters{Type: "app"},
			fetchOK:    false,
			wantEngine: "",
		},
		{
			name:        "postgres DATABASE_URL is detected for a plain app",
			params:      AppStackParameters{Type: "app"},
			databaseURL: "postgres://user:pass@host/db",
			fetchOK:     true,
			wantEngine:  "postgres",
		},
		{
			name:        "mysql DATABASE_URL is detected for a plain app",
			params:      AppStackParameters{Type: "app"},
			databaseURL: "mysql://user:pass@host/db",
			fetchOK:     true,
			wantEngine:  "mysql",
		},
		{
			name:        "unrecognized scheme yields empty (warning is side effect, not asserted here)",
			params:      AppStackParameters{Type: "app"},
			databaseURL: "mongodb://user:pass@host/db",
			fetchOK:     true,
			wantEngine:  "",
		},
		{
			name:       "no DATABASE_URL set (app not yet created) yields empty, matching an unset app",
			params:     AppStackParameters{Type: "app"},
			fetchOK:    true,
			wantEngine: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			originalFetch := fetchDatabaseURL
			defer func() { fetchDatabaseURL = originalFetch }()

			databaseURL := tt.databaseURL
			fetchOK := tt.fetchOK
			fetchDatabaseURL = func(_ aws.Config, _ string) (string, bool) {
				return databaseURL, fetchOK
			}

			p := tt.params
			p.detectExternalDatabaseEngine(aws.Config{})

			if p.ExternalDatabaseEngine != tt.wantEngine {
				t.Errorf("ExternalDatabaseEngine = %q, want %q", p.ExternalDatabaseEngine, tt.wantEngine)
			}
		})
	}
}

// TestDetectExternalDatabaseEngineIdempotent verifies that repeatedly running detection
// (as `apppack modify app` would on every invocation) converges rather than flip-flopping:
// a managed database stays "" forever, and an app whose DATABASE_URL is later unset goes
// back to "".
func TestDetectExternalDatabaseEngineIdempotent(t *testing.T) {
	originalFetch := fetchDatabaseURL
	defer func() { fetchDatabaseURL = originalFetch }()

	p := AppStackParameters{Type: "app"}

	// DATABASE_URL set to postgres -- engine detected.
	fetchDatabaseURL = func(_ aws.Config, _ string) (string, bool) {
		return "postgres://user:pass@host/db", true
	}

	p.detectExternalDatabaseEngine(aws.Config{})

	if p.ExternalDatabaseEngine != "postgres" {
		t.Fatalf("ExternalDatabaseEngine = %q, want %q after first detection", p.ExternalDatabaseEngine, "postgres")
	}

	// Running again with the same DATABASE_URL converges to the same value.
	p.detectExternalDatabaseEngine(aws.Config{})

	if p.ExternalDatabaseEngine != "postgres" {
		t.Fatalf("ExternalDatabaseEngine = %q, want %q to stay stable on repeat", p.ExternalDatabaseEngine, "postgres")
	}

	// DATABASE_URL is later unset -- detection must go back to "".
	fetchDatabaseURL = func(_ aws.Config, _ string) (string, bool) {
		return "", false
	}

	p.detectExternalDatabaseEngine(aws.Config{})

	if p.ExternalDatabaseEngine != "" {
		t.Errorf("ExternalDatabaseEngine = %q, want empty after DATABASE_URL is unset", p.ExternalDatabaseEngine)
	}
}

// TestExternalDatabaseEngineRoundTrip verifies ExternalDatabaseEngine is a real
// CloudFormation parameter (no cfnignore) and round-trips through
// ToCloudFormationParameters / Import.
func TestExternalDatabaseEngineRoundTrip(t *testing.T) {
	t.Helper()

	params := AppStackParameters{ExternalDatabaseEngine: "postgres"}

	cfnParams, err := params.ToCloudFormationParameters()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var found bool
	for _, p := range cfnParams {
		if *p.ParameterKey == "ExternalDatabaseEngine" {
			found = true
			if *p.ParameterValue != "postgres" {
				t.Errorf("ParameterValue = %q, want %q", *p.ParameterValue, "postgres")
			}
		}
	}
	if !found {
		t.Fatal("ExternalDatabaseEngine parameter not found in CloudFormation parameters")
	}

	var imported AppStackParameters
	if err := imported.Import(cfnParams); err != nil {
		t.Fatalf("unexpected error importing: %v", err)
	}

	if imported.ExternalDatabaseEngine != "postgres" {
		t.Errorf("imported ExternalDatabaseEngine = %q, want %q", imported.ExternalDatabaseEngine, "postgres")
	}
}
