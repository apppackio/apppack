.PHONY: fmt
fmt:
	gofumpt -w .
	gci write . --skip-generated
	golangci-lint run --fix

.PHONY: test
test:
	go test ./... -cover -coverprofile=coverage.out && go tool cover -func=coverage.out

.PHONY: lint
lint:
	golangci-lint run
