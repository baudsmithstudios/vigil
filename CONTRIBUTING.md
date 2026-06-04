# Contributing to Vigil

Thanks for your interest in contributing! Vigil is a small project and contributions are welcome.

## Getting started

1. Fork the repo and clone your fork
2. Make sure you have the Go version required by `go.mod` installed
3. Run `go test ./...` to verify everything passes
4. Create a branch for your change

## Submitting changes

- Open a pull request against `main`
- Keep changes focused — one feature or fix per PR
- Include tests for new functionality
- Make sure `go test ./...` and `go vet ./...` pass before submitting
- CI also runs `govulncheck`; you can check locally with `go run golang.org/x/vuln/cmd/govulncheck@latest ./...`

## Reporting bugs

Open an issue with:
- What you expected to happen
- What actually happened
- Steps to reproduce
- Your environment (OS, Go version, Docker version if relevant)

## Code style

- Match the style of surrounding code
- Keep it simple — no over-engineering
- Run `gofmt` before committing

## Questions?

Open an issue. There are no bad questions.
