package cmd

import (
	"testing"

	"github.com/apppackio/apppack/ui/uitest"
	"github.com/aws/aws-sdk-go-v2/aws"
	ecstypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"
	"github.com/charmbracelet/huh"
)

func TestFormatTaskSize(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		cpu  *string
		mem  *string
		want string
	}{
		{
			name: "quarter cpu 512 MB",
			cpu:  aws.String("256"),
			mem:  aws.String("512"),
			want: "0.25 vCPU / 512 MB",
		},
		{
			name: "half cpu 1 GB",
			cpu:  aws.String("512"),
			mem:  aws.String("1024"),
			want: "0.5 vCPU / 1 GB",
		},
		{
			name: "1 cpu 2 GB",
			cpu:  aws.String("1024"),
			mem:  aws.String("2048"),
			want: "1 vCPU / 2 GB",
		},
		{
			name: "2 cpu 4 GB",
			cpu:  aws.String("2048"),
			mem:  aws.String("4096"),
			want: "2 vCPU / 4 GB",
		},
		{
			name: "16 cpu 32 GB",
			cpu:  aws.String("16384"),
			mem:  aws.String("32768"),
			want: "16 vCPU / 32 GB",
		},
		{
			name: "memory not divisible by 1 GB stays in MB",
			cpu:  aws.String("256"),
			mem:  aws.String("768"),
			want: "0.25 vCPU / 768 MB",
		},
		{
			name: "nil cpu returns empty string",
			cpu:  nil,
			mem:  aws.String("1024"),
			want: "",
		},
		{
			name: "nil memory returns empty string",
			cpu:  aws.String("512"),
			mem:  nil,
			want: "",
		},
		{
			name: "invalid cpu returns empty string",
			cpu:  aws.String("notanumber"),
			mem:  aws.String("1024"),
			want: "",
		},
		{
			name: "invalid memory returns empty string",
			cpu:  aws.String("512"),
			mem:  aws.String("notanumber"),
			want: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			task := &ecstypes.Task{
				Cpu:    tc.cpu,
				Memory: tc.mem,
			}

			got := formatTaskSize(task)
			if got != tc.want {
				t.Errorf("formatTaskSize() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestShellTaskSelectForm_SelectFirst(t *testing.T) {
	options := []huh.Option[int]{
		huh.NewOption("web: abc123", 0),
		huh.NewOption("worker: def456", 1),
		huh.NewOption("web: ghi789", 2),
	}

	form, idxPtr := ShellTaskSelectForm(options)
	tm := uitest.RunForm(t, form)
	uitest.SelectFirst(tm)
	uitest.WaitDone(t, tm)

	if *idxPtr != 0 {
		t.Errorf("expected index 0, got %d", *idxPtr)
	}
}

func TestShellTaskSelectForm_SelectSecond(t *testing.T) {
	options := []huh.Option[int]{
		huh.NewOption("web: abc123", 0),
		huh.NewOption("worker: def456", 1),
		huh.NewOption("web: ghi789", 2),
	}

	form, idxPtr := ShellTaskSelectForm(options)
	tm := uitest.RunForm(t, form)
	uitest.SelectNth(tm, 1)
	uitest.WaitDone(t, tm)

	if *idxPtr != 1 {
		t.Errorf("expected index 1, got %d", *idxPtr)
	}
}

func TestShellTaskSelectForm_SelectLast(t *testing.T) {
	options := []huh.Option[int]{
		huh.NewOption("web: abc123", 0),
		huh.NewOption("worker: def456", 1),
		huh.NewOption("web: ghi789", 2),
	}

	form, idxPtr := ShellTaskSelectForm(options)
	tm := uitest.RunForm(t, form)
	uitest.SelectNth(tm, 2)
	uitest.WaitDone(t, tm)

	if *idxPtr != 2 {
		t.Errorf("expected index 2, got %d", *idxPtr)
	}
}
