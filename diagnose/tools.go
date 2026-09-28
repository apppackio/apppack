package diagnose

import (
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
)

// MaxToolResultBytes caps a single tool result. A crashlooping service can emit
// megabytes of logs; without a cap one tool call would exhaust the budget.
const MaxToolResultBytes = 60000

// ErrUnknownTool is returned when the model names a tool outside the allowlist.
var ErrUnknownTool = errors.New("unknown tool")

// Tool is one read-only capability offered to the model.
//
// Every Invoke implementation MUST wrap a read-only AWS call. This is the
// security boundary: the model can reach nothing this registry does not expose.
type Tool struct {
	Name        string
	Description string
	Schema      map[string]any
	Invoke      func(args map[string]any) (string, error)
}

// Registry is the allowlist of tools the model may call.
type Registry struct {
	tools map[string]Tool
	order []string
}

// NewRegistry builds a registry from an ordered list of tools.
func NewRegistry(tools []Tool) *Registry {
	r := &Registry{tools: map[string]Tool{}}

	for _, t := range tools {
		r.tools[t.Name] = t
		r.order = append(r.order, t.Name)
	}

	sort.Strings(r.order)

	return r
}

// Names returns the registered tool names, sorted.
func (r *Registry) Names() []string {
	return append([]string(nil), r.order...)
}

// Tools returns the registered tools in name order.
func (r *Registry) Tools() []Tool {
	out := make([]Tool, 0, len(r.order))
	for _, n := range r.order {
		out = append(out, r.tools[n])
	}

	return out
}

// Call invokes a tool by name. An unknown name returns ErrUnknownTool; the
// caller turns that into an error tool result so the loop can continue.
func (r *Registry) Call(name string, args map[string]any) (string, error) {
	t, ok := r.tools[name]
	if !ok {
		return "", fmt.Errorf("%w: %q", ErrUnknownTool, name)
	}

	out, err := t.Invoke(args)
	if err != nil {
		// Cap error text too: MaxToolResultBytes exists to bound the cost of a
		// single tool call, and an AWS error can carry an arbitrarily large
		// request/response body.
		return "", errors.New(Truncate(err.Error(), MaxToolResultBytes))
	}

	return Truncate(out, MaxToolResultBytes), nil
}

// ValidateChoice reads a string argument and checks it against an allowlist.
// Arguments are never interpolated into an AWS call unvalidated.
func ValidateChoice(args map[string]any, key string, allowed []string) (string, error) {
	if len(allowed) == 0 {
		return "", fmt.Errorf(
			"cannot look up %s: no services are running for this app, because it has never completed a successful release",
			key,
		)
	}

	raw, ok := args[key]
	if !ok {
		return "", fmt.Errorf("missing required argument %q (must be one of: %s)", key, strings.Join(allowed, ", "))
	}

	s, ok := raw.(string)
	if !ok {
		return "", fmt.Errorf("argument %q must be a string (must be one of: %s)", key, strings.Join(allowed, ", "))
	}

	for _, a := range allowed {
		if s == a {
			return s, nil
		}
	}

	return "", fmt.Errorf("invalid %s %q: must be one of: %s", key, s, strings.Join(allowed, ", "))
}

// ValidateInt reads an optional numeric argument, falling back to def and
// clamping to [minV, maxV]. JSON numbers arrive as float64.
func ValidateInt(args map[string]any, key string, def, minV, maxV int) (int, error) {
	raw, ok := args[key]
	if !ok || raw == nil {
		return def, nil
	}

	var v int

	switch n := raw.(type) {
	case float64:
		v = int(n)
	case int:
		v = n
	default:
		return 0, fmt.Errorf("argument %q must be a number", key)
	}

	if v < minV {
		v = minV
	}

	if v > maxV {
		v = maxV
	}

	return v, nil
}

// Truncate caps a tool result, keeping the tail. Errors and stack traces
// appear at the end of a log, so the tail is the useful half.
func Truncate(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}

	return fmt.Sprintf(
		"[earlier output truncated: %d bytes omitted, showing the most recent %d bytes]\n%s",
		len(s)-maxBytes, maxBytes, s[len(s)-maxBytes:],
	)
}

// ToolDeps supplies the read-only data sources the tools wrap. Each field is
// a function so tools can be tested without AWS.
type ToolDeps struct {
	Services []string

	PhaseLog       func(phase string) (string, error)
	AppLogs        func(service string, sinceMinutes, limit int) (string, error)
	ECSEvents      func(service string) (string, error)
	DescribeTasks  func(service string) (string, error)
	TaskDefinition func(service string) (string, error)
}

func stringSchema(desc string, enum []string) map[string]any {
	p := map[string]any{"type": "string", "description": desc}
	if enum != nil {
		// Clone: this map is handed to callers outside the package via
		// Registry.Tools(), and enum aliases the live validation allowlist
		// (phaseNames or ToolDeps.Services). Mutating the schema in place
		// must never mutate the allowlist the validator checks against.
		p["enum"] = slices.Clone(enum)
	}

	return p
}

func objectSchema(props map[string]any, required []string) map[string]any {
	return map[string]any{
		"type":       "object",
		"properties": props,
		"required":   required,
	}
}

// BuildTools returns the complete, read-only tool allowlist.
//
// Adding a tool here requires updating TestRegistryAllowlist. Every tool must
// wrap a read-only AWS call.
func BuildTools(d ToolDeps) []Tool {
	return []Tool{
		{
			Name:        "get_phase_log",
			Description: "Read the full log for one build phase. Use this when a build, test, release, or postdeploy phase failed.",
			Schema: objectSchema(map[string]any{
				"phase": stringSchema("Which build phase's log to read.", phaseNames),
			}, []string{"phase"}),
			Invoke: func(args map[string]any) (string, error) {
				phase, err := ValidateChoice(args, "phase", phaseNames)
				if err != nil {
					return "", err
				}

				return d.PhaseLog(phase)
			},
		},
		{
			Name:        "get_app_logs",
			Description: "Read recent application logs for one service from CloudWatch. Use this to find startup errors, tracebacks, and health check failures.",
			Schema: objectSchema(map[string]any{
				"service":       stringSchema("Which service's logs to read.", d.Services),
				"since_minutes": map[string]any{"type": "integer", "description": "How far back to look, in minutes. Defaults to 60."},
				"limit":         map[string]any{"type": "integer", "description": "Maximum number of log lines. Defaults to 200, capped at 1000."},
			}, []string{"service"}),
			Invoke: func(args map[string]any) (string, error) {
				service, err := ValidateChoice(args, "service", d.Services)
				if err != nil {
					return "", err
				}

				since, err := ValidateInt(args, "since_minutes", 60, 1, 10080)
				if err != nil {
					return "", err
				}

				limit, err := ValidateInt(args, "limit", 200, 1, 1000)
				if err != nil {
					return "", err
				}

				return d.AppLogs(service, since, limit)
			},
		},
		{
			Name:        "get_ecs_events",
			Description: "Read recent ECS service events for one service. These show health check failures, task placement problems, and deployment progress.",
			Schema: objectSchema(map[string]any{
				"service": stringSchema("Which service's events to read.", d.Services),
			}, []string{"service"}),
			Invoke: func(args map[string]any) (string, error) {
				service, err := ValidateChoice(args, "service", d.Services)
				if err != nil {
					return "", err
				}

				return d.ECSEvents(service)
			},
		},
		{
			Name:        "describe_tasks",
			Description: "Describe the running and recently stopped ECS tasks for one service, including stop reasons, exit codes, and health status.",
			Schema: objectSchema(map[string]any{
				"service": stringSchema("Which service's tasks to describe.", d.Services),
			}, []string{"service"}),
			Invoke: func(args map[string]any) (string, error) {
				service, err := ValidateChoice(args, "service", d.Services)
				if err != nil {
					return "", err
				}

				return d.DescribeTasks(service)
			},
		},
		{
			Name:        "get_task_definition",
			Description: "Read the full ECS task definition for one service: image, command, resource limits, health check, and environment variables.",
			Schema: objectSchema(map[string]any{
				"service": stringSchema("Which service's task definition to read.", d.Services),
			}, []string{"service"}),
			Invoke: func(args map[string]any) (string, error) {
				service, err := ValidateChoice(args, "service", d.Services)
				if err != nil {
					return "", err
				}

				return d.TaskDefinition(service)
			},
		},
	}
}
