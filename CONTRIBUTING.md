# Contributing to ethernetipctl

Thank you for your interest in contributing to `ethernetipctl`! This document covers the guidelines for contributing.

## Getting Started

1. Fork the repository on GitHub
2. Clone your fork locally:
   ```sh
   git clone git@github.com:<your-username>/ethernetipctl.git
   cd ethernetipctl
   ```
3. Create a feature branch:
   ```sh
   git checkout -b feature/my-change
   ```

## Development Setup

### Prerequisites

- Go 1.25+ (see [go.mod](go.mod))
- [staticcheck](https://staticcheck.dev/), [golangci-lint](https://golangci-lint.run/) and [govulncheck](https://go.dev/doc/security/vuln/) for the checks
- No hardware: the test suite brings its own devices (see [Testing](#testing))

### Build & Test

```sh
# Run all checks (format, lint, vet, 32-bit typecheck, vulnerability scan, race tests, coverage gate)
make check

# Run tests with the race detector
make test

# Run tests with whole-module coverage (fails below 90%)
make coverage

# Build for the current platform (./bin/ethernetipctl); runs `make check` first
make build
```

Run `make` (or `make help`) for the full list of targets.

## Making Changes

### Code Style

- Follow standard Go conventions (`gofmt`, `go vet`)
- Keep functions focused and small
- Use meaningful variable and function names
- Add comments only where the logic isn't self-evident; comments end with a period (enforced by the linter)
- Classify every failure: an error that reaches the user is an `*errs.Error` with a kind and a stable code (`internal/errs`, `eip.WrapError`). A new code is part of the contract: add it to [docs/exitcodes.md](docs/exitcodes.md)
- Protocol logic belongs in go-ethernetip: the CLI parses flags, calls the library and renders results. If the library lacks something, add it there
- A command that changes a device needs `--yes`, validated before any connection is opened, and `--yes` never has an environment variable

### Formatting

`make fmt` formats with the `gofmt` of the Go version in `go.mod`, which is the version CI uses, not necessarily the one you have installed. `make fmt-check` (part of `make check`) fails if either that `gofmt` or your local one would change a file: `gofmt` output differs slightly between Go releases, and a file the two disagree on fails CI. If they disagree, restructure the code (for example, move trailing comments onto their own lines) rather than picking a side.

### Commit Messages

Use clear, concise commit messages:

```
component: short description

Optional longer explanation of the change, why it was made,
and any relevant context.
```

Examples:
- `io: write only changed samples with --changes`
- `read: decode STRING2 values`
- `docs: document the adapter model`

### Testing

- Add tests for new functionality in the appropriate `_test.go` file
- Anything that talks to a device runs against a real go-ethernetip server on a free loopback port: `startFixture` in [`cmd/helper_test.go`](cmd/helper_test.go) serves OpENer's sample assemblies, an output echoed to the input, a Logix tag database, and counts sessions and requests (which proves that a change without `--yes` sends nothing). Command tests run the real command tree through `cmd.Run` and assert on stdout, stderr and the exit code
- The JSON output of representative commands is pinned by golden files in `cmd/testdata/golden`; after an intended change, regenerate them with `go test ./cmd -run TestGoldenJSON -update` and review the diff
- Assert behaviour (output, exit code, the device's data), not just that the code ran
- Tests must not write files into the repository: use `t.TempDir()`
- Do not assume timing a device does not promise: wait for a condition with a deadline (`eventually`) instead of sleeping
- Keep tests fast and race-clean; CI runs them with `-race`
- Whole-module statement coverage must stay at or above 90% (`make coverage`)
- If your change affects what goes on the wire, verify against a real device or another stack when possible (OpENer, or the containers of [ethernetip-interop](https://github.com/otfabric/ethernetip-interop)), and say in the pull request what you tested against

### Documentation

When you change commands, flags, output or exit codes, update:

- The command's `Short` / `Long` / `Example` help text
- [README.md](README.md), including the environment variable table for new flags
- [docs/json-output.md](docs/json-output.md) and [docs/exitcodes.md](docs/exitcodes.md) — `--format json` and exit codes are a stable contract for scripts
- [docs/adapter-config.md](docs/adapter-config.md) and [testdata/adapter.yaml](testdata/adapter.yaml) when the adapter model changes
- [RELEASE.md](RELEASE.md), calling out anything that breaks existing usage: a `## vX.Y.Z` section at the top, with `**Date:**` and `**Previous release:**` lines, ending with a `---` line. The release workflow publishes the text from `## <tag>` to the next `---` as the release notes, so there is never an Unreleased section

## Submitting Changes

1. Push your branch to your fork:
   ```sh
   git push origin feature/my-change
   ```
2. Open a Pull Request against the `main` branch
3. Describe what your change does and why
4. Reference any related issues

For larger changes, please open an issue first to discuss the approach.

## Reporting Issues

- Use GitHub Issues to report bugs or suggest features
- Include the output of `ethernetipctl version` in bug reports
- Provide the exact command that triggered the issue, along with the error output; `--debug` adds go-ethernetip's protocol log
- Mention the device (vendor, product, firmware revision: `ethernetipctl identify` shows them) and whether a route or a Class 3 connection was used
- Report security vulnerabilities privately: see [SECURITY.md](SECURITY.md)

## Project Structure

```
cmd/                   Root command, version, completion, Run
cmd/device/            Commands on one device (discover, identify, status, services, interfaces, read, write, objects, object, reset, session, forward-open, forward-close, connections, io, assembly, diagnose)
cmd/tag/               Rockwell Logix tag commands
cmd/eds/               EDS commands
cmd/adapter/           Simulated adapter
internal/eip/          What the commands do with go-ethernetip, error classification
internal/adapter/      The adapter model and server
internal/config/       Configuration types, flag and environment binding, host parsing
internal/format/       Output formatting (text, JSON, table, streaming)
internal/types/        Output data models
internal/runner/       Command wiring, formatted and streaming runs, fatal rendering
internal/errs/         Typed errors, stable codes, exit-code mapping
internal/cli/          Shared Cobra helpers
docs/                  Exit codes, JSON output contract, adapter model reference
```

## License

By contributing to `ethernetipctl`, you agree that your contributions will be licensed under the [MIT License](LICENSE).
