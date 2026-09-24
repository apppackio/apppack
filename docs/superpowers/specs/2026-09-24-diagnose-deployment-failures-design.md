# Design: `apppack diagnose`

**Date:** 2026-09-24
**Status:** Approved, pending implementation plan

## Problem

Diagnosing a failed deployment is the biggest friction point in AppPack, especially
for a user's first deploy. The information needed is always available -- ECS service
events, task stop reasons, application logs, build logs, the task definition -- but
finding the relevant piece requires knowing where to look and what a given symptom
implies. A health check failure sends you to the app logs; a task that never starts
sends you to the task definition's command; a build failure sends you to the
CodeBuild log.

This is pattern matching over structured evidence, which an LLM does well given the
right instructions and access to the same data a human would read.

## Goal

A `diagnose` command that gathers evidence from the customer's AWS account, asks a
model on Amazon Bedrock to identify the likely cause, and prints a diagnosis.

Success looks like: a user whose first deploy failed runs one command and gets a
specific, actionable answer ("your web process isn't binding to `$PORT`; the health
check on `/` timed out after 30s and ECS killed the task") instead of a generic
pointer to the troubleshooting guide.

### Non-goals

- Fixing anything. The command is strictly read-only and never modifies state.
- Replacing `events`, `logs`, or `build status`. It complements them.
- Streaming output. A single diagnosis behind a spinner is sufficient; revisit only
  if latency proves painful in practice.

## Constraints

These are requirements, not preferences.

1. **Strictly read-only.** No tool available to the model may modify the customer's
   environment. This is enforced structurally, not by instructing the model.
2. **No secret values.** The model may see which config variables are *defined*.
   It must never receive their values.
3. **Nothing leaves the customer's AWS account**, consistent with the trust boundary
   documented in `CLAUDE.md`. Bedrock is called with the customer's own assumed-role
   credentials, in the customer's own account.
4. **Bounded cost.** The customer pays for tokens. A confused model must not be able
   to loop indefinitely.

## Decisions

Recorded with reasoning, since several were non-obvious.

### Bedrock runs in the customer's account

The CLI assumes a role in the customer's account and makes all AWS calls with those
credentials; Bedrock is no different. The alternative -- AppPack calling Bedrock on
the customer's behalf -- would work on day one for every customer with no IAM change,
but would mean shipping application logs and config out of their account. That trades
away the property that makes the rest of the CLI trustworthy.

The cost of this decision is a rollout dependency: the AppPack-managed IAM role needs
new permissions before the command works. See [Rollout](#rollout).

Note for precision: Bedrock on-demand inference executes in AWS-operated model
deployment accounts, not literally inside the customer's account. This is equally
true of ECS, CloudWatch Logs, and SSM. Bedrock introduces no boundary the CLI does
not already cross on every command, and AWS documents that Bedrock does not retain
prompts or completions, does not use them for training, and does not share them with
model providers.

### Evidence is pulled by the model, not pushed

The model gets a small allowlist of read-only tools and requests what it needs, rather
than receiving one fixed evidence bundle.

The alternative -- assembling a fixed bundle and making a single call -- is simpler
and has no tool surface at all. It was rejected because log volume makes it
impractical: a crashlooping service emits thousands of lines, so a fixed bundle either
truncates away the relevant ones or buries them. More fundamentally, the interesting
failures are exactly the ones where the first piece of evidence only tells you where
to look next. Following that thread is the entire value of the feature.

The security cost is acceptable because the tools are Go functions in the CLI. The
model cannot invoke anything outside the allowlist, cannot reach a shell, and cannot
make an arbitrary AWS call.

### Logs are sent unmodified; output is scrubbed

Application logs routinely contain secrets -- a traceback dumping settings, a failed
connection logging a full `DATABASE_URL`. Three options were considered:

- Send as-is. Justified because those logs already sit in CloudWatch in the same
  account, and Bedrock is another AWS service reached with the same credentials.
- Pattern-based redaction before sending. Cheap, partial, and mangles innocent lines.
- Value-matched redaction -- fetch the real config values and scrub occurrences.
  Most reliable, but requires reading every secret, contradicting constraint 2.

**Decision: send as-is, and scrub the model's output.** The realistic leak is not
Bedrock seeing a secret -- it already has the logs' contents by construction, and does
not retain them. The realistic leak is the *diagnosis* quoting a secret back and the
user pasting it into a GitHub issue. Mitigation therefore belongs on the output side.

This means redaction is best-effort, and the documentation must say so rather than
implying a guarantee.

### Model is configurable; Claude is the default

Bedrock's data-handling guarantees are uniform across model providers, so choosing a
model for security reasons would be superstition. The genuine differentiators are
cross-region routing, network path, and whether invocation logging is enabled -- none
of which depend on which model is selected.

The command therefore uses the **Converse API** (`bedrock-runtime`), which normalizes
requests and tool use across model families, and exposes `--model`. This lets a
customer who has vetted a specific model, or who wants a first-party Amazon model,
switch without a code change. The default is a current Claude model, chosen for
strength at reading stack traces and correlating events.

Tool use is not supported by every Bedrock model. An unsupported `--model` must fail
with a clear message rather than a confusing API error. Detection is by translating
the Converse API's own validation error, **not** by maintaining a hardcoded list of
compatible models -- such a list would go stale every time AWS adds a model, and would
wrongly reject models that gained tool support after the CLI was released.

### Region is derived from the app's region, and is not configurable

Current Claude models are invoked through cross-region inference profiles, which route
within a geography. The app's region determines the geography:

| App region | Inference profile geography |
|---|---|
| `us-*`  | `us`   |
| `eu-*`  | `eu`   |
| `ap-*`  | `apac` |

Any other region is an explicit "not supported" error. There is deliberately no
override: a silent fallback to another geography would ship an EU customer's
application logs to the US, which is precisely the surprise the trust boundary exists
to prevent. Only `us`, `eu`, and `apac` are supported.

### Invocation is always explicit

`build watch` prints a hint on failure pointing at `apppack diagnose`, but never
invokes it. Auto-running would spend the customer's money without consent. A hint at
the moment of failure keeps it discoverable without taking that decision away.

## Command surface

```
apppack -a my-app diagnose [<build-number>]

Flags:
  --model string   Bedrock model ID to use for diagnosis
```

With no argument, it inspects the most recent build. If no build has failed, it
diagnoses the app's current state instead -- "my app is down and I didn't just deploy"
is the same investigation using the same tools.

## Architecture

A new `diagnose/` package; `cmd/diagnose.go` stays thin, matching the repo's existing
command structure.

| File | Responsibility |
|---|---|
| `diagnose/evidence.go` | Read-only evidence gathering, wrapping existing `App` methods |
| `diagnose/tools.go`    | Tool registry, JSON schemas, argument validation |
| `diagnose/bedrock.go`  | Converse client, region mapping, the tool loop |
| `diagnose/redact.go`   | Output scrubbing |
| `diagnose/prompt.go`   | System prompt and phase-specific guidance |

The package takes a `*app.App` and constructs no AWS clients other than
`bedrock-runtime`, inheriting the authenticated, region-scoped session.

### Preloaded context

Sent in the first message, because it is small, structured, and always relevant:

- App name, region, and build number (build number omitted when diagnosing current
  state with no recent build)
- The six phase states from `BuildStatus` (Build, Test, Finalize, Release,
  Postdeploy, Deploy), each `succeeded` / `failed` / `started`. Omitted entirely when
  there is no build to inspect; the prompt must handle its absence rather than
  receiving empty phases, which would read as "nothing failed"
- Service list from `GetServices()`
- Config variable **names** only
- Per-service task definition summary: image, command, CPU/memory, health check
  configuration, and environment variable **names**

### Tools

| Tool | Backed by | Validation |
|---|---|---|
| `get_phase_log(phase)` | `S3Log` on `BuildPhaseDetail.Logs` | `phase` in the six-phase enum |
| `get_app_logs(service, since, limit)` | CloudWatch on `Settings.LogGroup.Name` | `service` in `GetServices()`; `since` bounded; `limit` capped |
| `get_ecs_events(service)` | `App.GetECSEvents` | `service` in `GetServices()` |
| `describe_tasks(service)` | `App.DescribeTasks` | `service` in `GetServices()`; returns stop reasons, exit codes, health status |
| `get_task_definition(service)` | `App.TaskDefinition` | `service` in `GetServices()` |

The preload/pull split is the core design idea: the model receives the shape of the
problem for free and spends tokens only on logs it has a reason to read. The failed
phase drives its first move -- a Build failure points at the CodeBuild log in S3, a
Deploy failure at ECS events and task stop reasons.

## Security model

Six invariants. Each is enforced by construction and covered by a test; none relies on
the model following instructions.

1. **No mutation is possible.** Every tool wraps a read-only AWS call. A test asserts
   the registry contains exactly the allowlisted tool names, so adding a tool requires
   deliberately editing that test.

2. **No secret values are read.** Config names come from `GetParametersByPath` with
   `WithDecryption: false`; `Value` is discarded at the boundary. `App.GetConfig()`
   is never called from this package -- it sets `WithDecryption: true`
   (`app/utils.go`) and returns plaintext secrets. A test asserts decryption is never
   requested. Note this is enforced in CLI code, not by IAM: the role *can* decrypt,
   we simply never ask it to.

3. **Arguments are validated, never interpolated.** Service names are checked against
   `GetServices()`, phases against an enum, numerics against bounds. Nothing reaches
   an AWS call unvalidated. Since the session is scoped to a single app's role,
   cross-app access is additionally impossible at the IAM layer.

4. **Log content is untrusted input.** Anyone who can write to the application's logs
   can write text shaped like instructions. Log content is wrapped in delimiters and
   labeled as data, with an explicit instruction that text inside is never an
   instruction. This is mitigation, not prevention. The actual guarantee comes from
   invariant 1: a successful injection yields a wrong answer, never an action.

5. **Output is scrubbed.** A redaction pass over the model's response runs before
   printing, catching credential-shaped strings: assignments whose key matches
   `KEY|TOKEN|SECRET|PASSWORD|CREDENTIAL`, connection-string userinfo
   (`proto://user:pass@host`), AWS access key IDs, and JWTs. The system prompt also
   instructs the model not to echo values that look like credentials.

6. **Cost is bounded.** A hard cap on tool-call rounds and a total token budget.

## Failure modes

Every one of these gets a specific, actionable message rather than a raw AWS error.

| Condition | Message |
|---|---|
| `AccessDeniedException` on invoke, role lacks Bedrock permissions | Instruct the user to upgrade their stack: `apppack upgrade app <name>`, or `apppack upgrade pipeline <name>` when `a.Pipeline` is true |
| `AccessDeniedException`, model access not enabled | Point at the Bedrock console's Model access page for the resolved region |
| App region outside `us-*` / `eu-*` / `ap-*` | Name the supported geographies |
| `--model` does not support tool use | State that explicitly |
| `ThrottlingException` | Retry with backoff, then a clear message |

The two `AccessDeniedException` cases are hard to tell apart from the error alone;
the message should cover both possibilities in priority order, leading with the stack
upgrade since that is the expected cause during rollout.

This error path is load-bearing rather than polish: the CLI will ship before the IAM
change reaches every account, so for a period the permission error *is* the feature's
front door.

## Testing

Table-driven, per the repo's conventions, with `testify` assertions.

- Redaction patterns, including near-misses that must **not** be mangled
- Argument validation rejecting out-of-enum services and phases, and out-of-bounds
  numerics
- The registry allowlist assertion (invariant 1)
- The no-decryption assertion (invariant 2)
- Iteration-cap and token-budget enforcement
- Region-to-geography mapping, including the unsupported-region error

The Bedrock client sits behind an interface so the tool loop is fully testable without
network access.

## Rollout

The AppPack-managed IAM role needs `bedrock:InvokeModel` on both the inference profile
ARN and the underlying foundation model ARNs in every region the profile can route to.

Those CloudFormation templates live in the `apppack-cloudformations` repository, not
here (`stacks/constants.go` resolves them from
`s3.amazonaws.com/apppack-cloudformations/`). This CLI work can therefore land and
ship independently, provided the permission error path is good -- which is why it is
specified above rather than left to implementation.

Customers must also enable model access for the chosen model in the resolved region.
That is a one-click grant in the Bedrock console and cannot be automated from the CLI.

## Open items for implementation

Deliberately unresolved here, to be settled against live documentation rather than
baked in from potentially stale knowledge:

- The exact current Bedrock inference profile IDs and the default Claude model ID.
  Verify against AWS documentation at implementation time.
- Which `bedrock-runtime` SDK module version to add to `go.mod`.
