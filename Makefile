.PHONY: all build clean test lint run-owl run-owld install-hooks

# Build parameters
BIN_DIR := bin
OWL_BIN := $(BIN_DIR)/owl
OWLD_BIN := $(BIN_DIR)/owld
GO_FILES := $(shell find . -name '*.go' -type f)

# Default target
all: build

# Build the binaries
build: $(OWL_BIN) $(OWLD_BIN)

$(OWL_BIN): $(GO_FILES)
	@echo "Building owl..."
	@mkdir -p $(BIN_DIR)
	@go build -o $(OWL_BIN) ./cmd/owl

$(OWLD_BIN): $(GO_FILES)
	@echo "Building owld..."
	@mkdir -p $(BIN_DIR)
	@go build -o $(OWLD_BIN) ./cmd/owld

# Run the client
run-owl: build
	@./$(OWL_BIN)

# Run the daemon
run-owld: build
	@./$(OWLD_BIN)

# Test the project
test:
	@echo "Running tests..."
	@go test -v -race ./...

# Lint the project
lint:
	@echo "Running linter..."
	@golangci-lint run

# Install git hooks
install-hooks:
	@echo "Installing git hooks..."
	@git config core.hooksPath .githooks
	@chmod +x .githooks/*
	@echo "Git hooks installed successfully."

# Clean build artifacts
clean:
	@echo "Cleaning..."
	@rm -rf $(BIN_DIR)
	@go clean
