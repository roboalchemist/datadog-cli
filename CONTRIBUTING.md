# Contributing

1. Fork the repository
2. Create a feature branch: `git checkout -b my-feature`
3. Make your changes
4. Run tests: `make check`
5. Submit a pull request

## Development

```bash
make deps          # Install dependencies
make build         # Build binary
make test          # Smoke tests
make test-unit     # Unit tests (90%+ coverage)
make test-integration  # Integration tests
make lint          # Lint
make check         # All of the above
```

## Code Style

- `go fmt` for formatting
- `golangci-lint` for linting
- Tests for all new features
