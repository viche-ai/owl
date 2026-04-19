.PHONY: all build clean fmt fmt-check test lint run-owl run-owld install-hooks dist distclean

# Build parameters
BIN_DIR := bin
DIST_DIR := dist
OWL_BIN := $(BIN_DIR)/owl
OWLD_BIN := $(BIN_DIR)/owld
GO_FILES := $(shell find . -name '*.go' -type f)
VERSION ?= $$(git describe --tags --always --dirty 2>/dev/null | sed 's/^v//')

# Default target
all: build

# Build the binaries
build: $(OWL_BIN) $(OWLD_BIN)

$(OWL_BIN): $(GO_FILES)
	@echo "Building owl..."
	@mkdir -p $(BIN_DIR)
	@go build -ldflags="-s -w" -o $(OWL_BIN) ./cmd/owl

$(OWLD_BIN): $(GO_FILES)
	@echo "Building owld..."
	@mkdir -p $(BIN_DIR)
	@go build -ldflags="-s -w" -o $(OWLD_BIN) ./cmd/owld

# Run the client
run-owl: build
	@./$(OWL_BIN)

# Run the daemon
run-owld: build
	@./$(OWLD_BIN)

# Run gofmt in-place
fmt:
	@echo "Formatting Go files..."
	@gofmt -w $(GO_FILES)

# Verify Go formatting
fmt-check:
	@echo "Checking Go formatting..."
	@UNFORMATTED="$$(gofmt -l $(GO_FILES))"; \
	if [ -n "$$UNFORMATTED" ]; then \
		echo "The following files need gofmt:"; \
		echo "$$UNFORMATTED"; \
		exit 1; \
	fi

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

# Build release distribution tarballs for current platform
dist: build
	@echo "Creating distribution archive..."
	@mkdir -p $(DIST_DIR)
	@ARCH="$$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/;s/armv7l/arm/')"
	@OS=$$(uname -s | tr '[:upper:]' '[:lower:]')
	@ARCHIVE="owl_$(VERSION)_$${OS}_$${ARCH}.tar.gz"
	@tar -czf "$(DIST_DIR)/$${ARCHIVE}" -C $(BIN_DIR) owl owld
	@echo "Created $(DIST_DIR)/$${ARCHIVE}"

# Clean dist directory
distclean:
	@echo "Cleaning dist..."
	@rm -rf $(DIST_DIR)
