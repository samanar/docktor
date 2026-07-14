.PHONY: build run test clean fmt vet install tidy help

APP_NAME   := docktor
CMD_DIR    := ./cmd/docktor
BIN_DIR    := ./bin
GO         := go
GOFLAGS    :=

# Default target
.DEFAULT_GOAL := help

## build: compile the binary
build:
	@mkdir -p $(BIN_DIR)
	$(GO) build $(GOFLAGS) -o $(BIN_DIR)/$(APP_NAME) $(CMD_DIR)

## run: build and run the app
run:
	$(GO) run $(GOFLAGS) $(CMD_DIR)

## test: run all tests
test:
	$(GO) test $(GOFLAGS) ./...

## test-verbose: run tests with verbose output
test-verbose:
	$(GO) test $(GOFLAGS) -v ./...

## coverage: run tests with coverage report
coverage:
	$(GO) test $(GOFLAGS) -coverprofile=coverage.out ./...
	$(GO) tool cover -html=coverage.out -o coverage.html

## fmt: format all Go source files
fmt:
	$(GO) fmt ./...

## vet: run go vet on all packages
vet:
	$(GO) vet ./...

## tidy: tidy module dependencies
tidy:
	$(GO) mod tidy

## clean: remove build artifacts
clean:
	rm -rf $(BIN_DIR)
	rm -f coverage.out coverage.html

## install: install binary to $GOPATH/bin
install:
	$(GO) install $(GOFLAGS) $(CMD_DIR)

## lint: run golangci-lint (requires golangci-lint installed)
lint:
	golangci-lint run ./...

## help: show this help
help:
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@sed -n 's/^##/  /p' $(MAKEFILE_LIST) | column -t -s ':'
