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
3. AWS STS role assumption for temporary credentials
4. Session creation with proper region configuration

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