# Contributing to CCC (Claude Command Center)

Thank you for your interest in contributing to CCC! This document provides guidelines for contributing.

## Development Setup

```bash
# Clone the repository
git clone https://github.com/tuannvm/ccc.git
cd ccc

# Install Go dependencies
go mod download

# Build standard binary
go build -o ccc .

# Build with voice features (requires whisper.cpp and ffmpeg)
make deps
go build -tags voice -o ccc .
```

## Code Quality

All changes must pass the following checks:

### Linting
```bash
# Run golangci-lint locally before pushing
golangci-lint run
```

### Testing
```bash
# Run all tests with coverage
go test -v -race -coverprofile=coverage.txt -covermode=atomic ./...
```

### Go Module
```bash
# Ensure go.mod is tidy
go mod tidy
git diff go.mod go.sum  # Should be empty
```

## Making Changes

1. **Fork and Branch**: Create a feature branch from `main`
2. **Write Code**: Follow existing code style and patterns
3. **Test**: Add tests for new functionality
4. **Lint**: Run `golangci-lint run` and fix issues
5. **Commit**: Use clear commit messages (conventional commits preferred)
6. **Push**: Push to your fork
7. **PR**: Open a pull request

## Pull Request Process

### Before Opening a PR
- [ ] Tests pass locally
- [ ] Linting passes (`golangci-lint run`)
- [ ] `go mod tidy` produces no changes
- [ ] Commit messages follow conventional commits
- [ ] PR description describes the change and motivation

### PR Review
- All PRs require approval from a code owner
- Address review comments promptly
- Keep PRs focused and small

## Voice Features

The `voice` build tag enables transcription via whisper.cpp:
- Requires CGO
- Requires whisper.cpp submodule (`make deps`)
- Requires ffmpeg for audio decoding

## Development Workflow

```bash
# Standard development cycle
go build . && ./ccc
go test ./...
golangci-lint run

# Voice feature development
make deps
go build -tags voice . && ./ccc
```

## Dependencies

- **Go**: 1.24+
- **Voice features**: whisper.cpp, ffmpeg
- **Linting**: golangci-lint

## Getting Help

- Open an issue for bugs or feature requests
- Check existing issues first
- Provide minimal reproduction cases for bugs

## License

By contributing, you agree that your contributions will be licensed under the MIT License.
