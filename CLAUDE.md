# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Development Commands

### Building and Testing
```bash
# Format code
make fmt

# Run tests with coverage
make test

# Run linter
make lint

# Build the CLI (via go build)
go build -o apppack main.go
```

### Running Single Tests
```bash
# Run a specific test file
go test ./app -v

# Run a specific test function  
go test ./app -run TestSpecificFunction -v

# Run tests with coverage for a specific package
go test ./stacks -cover -coverprofile=coverage.out
```

## Architecture Overview

This is a Go CLI application for managing cloud infrastructure via AppPack.io. The codebase follows a modular, interface-based architecture:

### Trust Boundary

**AppPack.io services are contacted for exactly two things: authenticating the user, and fetching the list of apps and accounts that user can access.** Nothing else. Concretely, that is `auth.apppack.io` (Auth0 device code flow, token refresh, userinfo) and `api.apppack.io/apps` + `api.apppack.io/accounts`, which return role ARNs (`auth/auth.go`).

**After that, the CLI operates entirely within the customer's AWS account.** The CLI assumes the role it was given and every subsequent call — CloudFormation, ECS, CloudWatch Logs, SSM, DynamoDB, and anything else — is an AWS API call made with those temporary credentials. No app data, log content, config, or metrics is ever sent back to AppPack.io. (The only other outbound traffic is the self-update check against GitHub releases, which carries no account data.)

This has direct consequences for any new feature:

- Customer data (logs, events, config, metrics) never leaves the customer's AWS account. Do not add code paths that ship account data to AppPack.io or any other third party.
- New AWS service calls require the corresponding permissions on the AppPack-managed IAM role in the customer's account. That means a CloudFormation stack update must roll out before the feature works, so plan for graceful degradation when permissions are missing.
- Services must be available and enabled in the customer's account and region. Don't assume an opt-in service is ready — detect the failure and tell the user how to enable it.

### Core Components

- **cmd/**: CLI commands using Cobra framework. Each command follows the pattern: authentication → AWS session → stack operations → user feedback
- **app/**: Application lifecycle management including ECS tasks, builds, configuration, and shell access via AWS Session Manager
- **stacks/**: Infrastructure abstraction layer with common Stack interface for CloudFormation operations across different resource types (clusters, databases, domains, etc.)
- **auth/**: OAuth-based authentication with Auth0, JWT token management, and AWS session creation via role assumption
- **bridge/**: AWS service integration wrappers for CloudFormation, EC2, Route53
- **aws/**: Low-level AWS SDK utilities for EventBridge, SSM

### Key Interfaces

The `Stack` interface in `stacks/interfaces.go` defines the contract for all infrastructure types:
- `GetParameters()`: CloudFormation parameter marshaling
- `StackName()`, `TemplateURL()`: Resource naming and template resolution  
- `AskQuestions()`: Interactive parameter collection
- Lifecycle hooks: `PostCreate()`, `PreDelete()`, `PostDelete()`

### Authentication Flow

1. OAuth device code flow for CLI-friendly auth (no browser required)
2. JWT tokens stored in filesystem cache with automatic refresh
3. AWS STS role assumption for temporary credentials (`auth/tokens.go`, `AssumeRoleWithWebIdentity`)
4. Session creation with proper region configuration

Steps 1-2 talk to AppPack.io; the app/account list lookup does too. From the role assumption onward, every call uses the customer's credentials against the customer's account and region. See [Trust Boundary](#trust-boundary).

### Stack Management Pattern

All infrastructure follows this lifecycle:
1. Parameter validation and collection (flags or interactive prompts)
2. CloudFormation template URL resolution
3. Changeset creation for preview
4. Stack creation/update with progress tracking
5. Post-deployment hooks for additional setup

## Testing Guidelines

- Use `github.com/stretchr/testify` for test assertions
- Mock AWS services using interfaces defined in the codebase
- Test files should be co-located with source files (`*_test.go`)
- Use table-driven tests for testing multiple scenarios

## Code Patterns

### Error Handling
Use the `checkErr()` function from `cmd/root.go` for consistent CLI error reporting with colored output.

### AWS Operations
Always use the session from the App struct (`app.Session`) for AWS SDK calls. The session includes proper authentication and region configuration.

### User Interaction
- Use `github.com/AlecAivazis/survey/v2` for interactive prompts
- Use `github.com/briandowns/spinner` for long-running operations
- Use `github.com/logrusorgru/aurora` for colored terminal output

### Stack Parameter Handling
When adding new stack types:
1. Define struct with CloudFormation parameter tags
2. Implement `Parameters` interface methods
3. Use reflection-based parameter conversion in `stacks/utils.go`

## Debugging

Enable debug logging with the `--debug` flag on any command. This will show detailed AWS API calls and internal operation logs via logrus.

## Release Process

The project uses GoReleaser with GitHub Actions:
1. Update CHANGELOG.md with release notes
2. Tag commit with version (e.g., `git tag -s v4.6.7`)
3. Push tag (`git push --tag`)
4. GoReleaser automatically builds and releases cross-platform binaries

### Writing CHANGELOG entries

**The CHANGELOG is for end users of the CLI, not for people reading the
code.** Write what changes for someone running `apppack`, and stop there.

Implementation detail belongs in the commit message and the PR, not here.
Internal work that a user cannot observe gets one short line, or no line at
all — "Upgraded the AWS SDK" is a complete entry. Do not explain which
packages moved, which functions were deleted, or why the refactor was
worthwhile.

Good:

* Upgraded the AWS SDK.
* `ps restart` command to restart your app's processes. By default it does a
  graceful, zero-downtime rolling restart; add `--force` to stop containers
  immediately.

Bad — these describe the codebase rather than the product:

* Replaced the reflection-based `newBlade` helper with the upstream
  `NewBladeWithConfig` constructor, removing `unsafe` from the codebase.
* Migrated `cmd/logs.go` and `cmd/db.go` off aws-sdk-go v1, dropping four
  staticcheck SA1019 warnings.

When a change *is* user-visible, lead with the symptom the user hit, not the
cause — see the 4.8.3 entries for the house style.