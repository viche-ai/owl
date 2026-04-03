# Contributing to Owl

First off, thank you for considering contributing to Owl! It's people like you that make Owl a great tool for the community.

## Code of Conduct

By participating in this project, you are expected to uphold our standards of open and welcoming collaboration. Please be respectful and constructive in all communications.

## How Can I Contribute?

### Reporting Bugs

- Ensure the bug was not already reported by searching on GitHub under [Issues](https://github.com/viche-ai/owl/issues).
- If you're unable to find an open issue addressing the problem, open a new one. Be sure to include a title and clear description, as much relevant information as possible, and a code sample or an executable test case demonstrating the expected behavior that is not occurring.

### Suggesting Enhancements

- Open a new issue with a clear title and description.
- Explain why this enhancement would be useful to most users.

### Pull Requests

1. Fork the repo and create your branch from `main`.
2. If you've added code that should be tested, add tests.
3. If you've changed APIs or behavior, update the documentation.
4. Ensure the test suite passes (`make test`).
5. Ensure the code passes the linter (`make lint`).
6. Issue that pull request!

## Local Development

Owl is built in Go. To set up your local development environment:

1. Install Go (1.21+ recommended).
2. Clone the repository: `git clone https://github.com/viche-ai/owl.git`
3. Run `make build` to build the `owl` client and `owld` daemon.

### Running during development

You can run the daemon in the background or in a separate terminal:
```bash
make run-owld
```

And run the TUI client in another terminal:
```bash
make run-owl
```

### Architecture Overview

Before contributing significant features, it helps to understand the architecture:

- **`cmd/`**: Entry points for the `owl` and `owld` binaries.
- **`internal/`**: The core application code.
  - `engine/`: Multi-provider LLM integrations (OpenAI, Anthropic, etc.).
  - `ipc/`: The RPC communication layer between the client and the daemon.
  - `tui/`: The Bubble Tea terminal UI.
  - `viche/`: The networking layer connecting to the Viche platform via WebSockets.

Please refer to `DESIGN.md` for deeper insights into the vision and constraints.

## Styleguides

### Git Commit Messages

- Use the present tense ("Add feature" not "Added feature").
- Use the imperative mood ("Move cursor to..." not "Moves cursor to...").
- Limit the first line to 72 characters or less.
- Reference issues and pull requests liberally after the first line.

### Go Code Style

We follow standard Go formatting and idioms. Please run `golangci-lint run` (or `make lint`) before submitting code.

Thank you!
