# Contributing to MDDB

Thank you for your interest in contributing to MDDB! This guide will help you get started.

Be respectful and constructive in all project interactions. Report conduct or
security concerns to **security@tradik.com**.

## Prerequisites

- [Go 1.26+](https://golang.org/dl/)
- [Make](https://www.gnu.org/software/make/)
- [Protocol Buffers compiler](https://grpc.io/docs/protoc-installation/) (only if modifying `proto/mddb.proto`)
- [Node.js 24+](https://nodejs.org/) (only for `mddb-panel` changes)

## Project Structure

MDDB is a monorepo with the following services:

| Directory | Description |
|-----------|-------------|
| `services/mddbd` | Core database server (HTTP/JSON + gRPC/Protobuf + MCP) |
| `services/mddb-cli` | Command-line client |
| `services/mddb-panel` | React web UI |
| `proto/` | Protocol Buffer definitions |
| `docs/` | Documentation |

## Getting Started

```bash
# Clone the repository
git clone https://github.com/tradik/mddb.git
cd mddb

# Build the server
cd services/mddbd
make build

# Run tests
make test

# Run linter
make lint
```

## Development Workflow

1. **Fork** the repository
2. **Create a feature branch** from `main`:
   ```bash
   git checkout -b feat/my-feature
   ```
3. **Make your changes**
4. **Add tests** for new functionality
5. **Regenerate proto code** if you modified `proto/mddb.proto`:
   ```bash
   make generate-proto
   ```
6. **Run checks**:
   ```bash
   make fmt
   make lint
   make test
   ```
7. **Commit** using conventional commits (see below)
8. **Push** and open a Pull Request

## Commit Convention

We use [Conventional Commits](https://www.conventionalcommits.org/):

| Prefix | Usage |
|--------|-------|
| `feat:` | New feature |
| `fix:` | Bug fix |
| `docs:` | Documentation changes |
| `chore:` | Maintenance, dependencies, CI |
| `refactor:` | Code refactoring (no behavior change) |
| `test:` | Adding or updating tests |
| `perf:` | Performance improvements |

Examples:
```
feat: add TTL support for documents
fix: resolve race condition in vector index
docs: update API reference with new endpoints
```

## Pull Request Guidelines

- Keep PRs focused on a single change
- Include a clear description of what changed and why
- Add or update tests as needed
- Update documentation for user-facing changes
- Ensure CI passes (tests, lint, build)

## Reporting Issues

- Use [GitHub Issues](https://github.com/tradik/mddb/issues) for bugs and feature requests
- For security vulnerabilities, see [SECURITY.md](SECURITY.md)

## License

By contributing, you agree that your contributions will be licensed under the [BSD 3-Clause License](LICENSE).
