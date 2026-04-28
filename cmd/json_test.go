package cmd

import (
	"encoding/json"
	"testing"
	"time"

	ecstypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"
)

func strPtr(s string) *string { return &s }

func TestTaskToJSON(t *testing.T) {
	t.Parallel()

	now := time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)
	taskARN := "arn:aws:ecs:us-east-1:123456789012:task/cluster/abc123"

	task := &ecstypes.Task{
		Tags: []ecstypes.Tag{
			{Key: strPtr("apppack:processType"), Value: strPtr("web")},
			{Key: strPtr("apppack:buildNumber"), Value: strPtr("42")},
		},
		Cpu:        strPtr("512"),
		Memory:     strPtr("1024"),
		LastStatus: strPtr("RUNNING"),
		StartedAt:  &now,
		TaskArn:    strPtr(taskARN),
	}

	tj, err := taskToJSON(task)
	if err != nil {
		t.Fatalf("taskToJSON returned error: %v", err)
	}

	if tj.Name != "web" {
		t.Errorf("expected Name=web, got %q", tj.Name)
	}

	if tj.Status != "running" {
		t.Errorf("expected Status=running, got %q", tj.Status)
	}

	const wantCPU = 0.5 // 512 / 1024
	if tj.CPU != wantCPU {
		t.Errorf("expected CPU=%.2f, got %.2f", wantCPU, tj.CPU)
	}

	if tj.Memory != "1024" {
		t.Errorf("expected Memory=1024, got %q", tj.Memory)
	}

	if tj.BuildNumber != "42" {
		t.Errorf("expected BuildNumber=42, got %q", tj.BuildNumber)
	}

	if tj.StartedAt == nil || !tj.StartedAt.Equal(now) {
		t.Errorf("expected StartedAt=%v, got %v", now, tj.StartedAt)
	}

	if tj.TaskARN != taskARN {
		t.Errorf("expected TaskARN=%q, got %q", taskARN, tj.TaskARN)
	}
}

func TestTaskToJSON_MissingTag(t *testing.T) {
	t.Parallel()

	task := &ecstypes.Task{
		Tags:       []ecstypes.Tag{},
		Cpu:        strPtr("256"),
		Memory:     strPtr("512"),
		LastStatus: strPtr("PENDING"),
		TaskArn:    strPtr("arn:aws:ecs:us-east-1:123:task/x"),
	}

	_, err := taskToJSON(task)
	if err == nil {
		t.Error("expected error for missing processType tag, got nil")
	}
}

func TestTaskToJSON_ShellProcess(t *testing.T) {
	t.Parallel()

	task := &ecstypes.Task{
		Tags: []ecstypes.Tag{
			{Key: strPtr("apppack:processType"), Value: strPtr("shell")},
			{Key: strPtr("apppack:buildNumber"), Value: strPtr("7")},
		},
		Cpu:        strPtr("1024"),
		Memory:     strPtr("2048"),
		LastStatus: strPtr("RUNNING"),
		TaskArn:    strPtr("arn:aws:ecs:us-east-1:123:task/shell"),
		StartedBy:  strPtr("user@example.com"),
	}

	tj, err := taskToJSON(task)
	if err != nil {
		t.Fatalf("taskToJSON returned error: %v", err)
	}

	if tj.StartedBy != "user@example.com" {
		t.Errorf("expected StartedBy=user@example.com, got %q", tj.StartedBy)
	}

	if tj.CPU != 1.0 {
		t.Errorf("expected CPU=1.0, got %.2f", tj.CPU)
	}
}

// TestTaskToJSON_JSONSchema verifies the stable JSON schema for the ps command output.
func TestTaskToJSON_JSONSchema(t *testing.T) {
	t.Parallel()

	now := time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC)
	task := &ecstypes.Task{
		Tags: []ecstypes.Tag{
			{Key: strPtr("apppack:processType"), Value: strPtr("worker")},
			{Key: strPtr("apppack:buildNumber"), Value: strPtr("99")},
		},
		Cpu:        strPtr("2048"),
		Memory:     strPtr("4096"),
		LastStatus: strPtr("RUNNING"),
		StartedAt:  &now,
		TaskArn:    strPtr("arn:aws:ecs:us-east-1:111:task/worker"),
	}

	tj, err := taskToJSON(task)
	if err != nil {
		t.Fatalf("taskToJSON returned error: %v", err)
	}

	out, err := json.Marshal(tj)
	if err != nil {
		t.Fatalf("json.Marshal returned error: %v", err)
	}

	// Verify all expected fields are present in the JSON output.
	var m map[string]interface{}
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatalf("json.Unmarshal returned error: %v", err)
	}

	for _, field := range []string{"name", "status", "cpu", "memory", "build_number", "started_at", "task_arn"} {
		if _, ok := m[field]; !ok {
			t.Errorf("expected field %q in JSON output, not found; got %s", field, string(out))
		}
	}

	// started_by should be omitted when empty.
	if _, ok := m["started_by"]; ok {
		t.Errorf("expected started_by to be omitted when empty, but it was present")
	}
}

// TestVersionInfoJSONSchema verifies the stable JSON schema for the version command output.
func TestVersionInfoJSONSchema(t *testing.T) {
	t.Parallel()

	info := versionInfo{
		Version:     "v4.6.7",
		Commit:      "abc1234",
		BuildDate:   "2024-01-01",
		Environment: "production",
	}

	out, err := json.Marshal(info)
	if err != nil {
		t.Fatalf("json.Marshal returned error: %v", err)
	}

	var m map[string]interface{}
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatalf("json.Unmarshal returned error: %v", err)
	}

	for _, field := range []string{"version", "commit", "build_date", "environment"} {
		if _, ok := m[field]; !ok {
			t.Errorf("expected field %q in version JSON output, not found; got %s", field, string(out))
		}
	}

	if m["version"] != "v4.6.7" {
		t.Errorf("expected version=v4.6.7, got %v", m["version"])
	}
}

// TestStackHumanizeJSONSchema verifies the stable JSON schema for the stacks command output.
func TestStackHumanizeJSONSchema(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		stack          stackHumanize
		wantClusterKey bool
	}{
		{
			name:           "with cluster",
			stack:          stackHumanize{Name: "my-app", Type: "app", Cluster: "production"},
			wantClusterKey: true,
		},
		{
			name:           "without cluster",
			stack:          stackHumanize{Name: "my-account", Type: "account"},
			wantClusterKey: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			out, err := json.Marshal(tc.stack)
			if err != nil {
				t.Fatalf("json.Marshal returned error: %v", err)
			}

			var m map[string]interface{}
			if err := json.Unmarshal(out, &m); err != nil {
				t.Fatalf("json.Unmarshal returned error: %v", err)
			}

			if m["name"] != tc.stack.Name {
				t.Errorf("expected name=%q, got %v", tc.stack.Name, m["name"])
			}

			if m["type"] != tc.stack.Type {
				t.Errorf("expected type=%q, got %v", tc.stack.Type, m["type"])
			}

			if tc.wantClusterKey {
				if m["cluster"] != tc.stack.Cluster {
					t.Errorf("expected cluster=%q, got %v", tc.stack.Cluster, m["cluster"])
				}
			} else {
				if _, ok := m["cluster"]; ok {
					t.Error("expected cluster to be omitted when empty")
				}
			}
		})
	}
}

// TestRootJSONPersistentFlag verifies that --json is a persistent flag on the root command.
func TestRootJSONPersistentFlag(t *testing.T) {
	t.Parallel()

	flag := rootCmd.PersistentFlags().Lookup("json")
	if flag == nil {
		t.Fatal("expected --json persistent flag on rootCmd, not found")
	}

	if flag.Shorthand != "j" {
		t.Errorf("expected shorthand -j, got %q", flag.Shorthand)
	}
}
