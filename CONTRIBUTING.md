# Contributing to Apollo Air-1 Exporter

Thank you for your interest in contributing to the Apollo Air-1 Exporter!

## Development Setup

1. Install Go 1.27 or newer
2. Clone the repository
3. Install development dependencies:
   ```bash
   make dev-setup
   ```

## Development Workflow

1. Create a new branch for your feature/fix
2. Make your changes
3. Run checks before committing:
   ```bash
   make check
   ```
4. Commit your changes with a descriptive message
5. Push your branch and create a pull request

## Code Style

- Follow Go standard formatting (enforced by `gofmt`)
- Ensure `go vet ./...` reports no issues
- Add tests for new functionality
- Update documentation as needed

## Testing

Run the test suite:
```bash
make test
```

Test with real devices:
```bash
go run .
```

In another terminal, query a device at scrape time:
```bash
curl --get 'http://localhost:9926/metrics' \
   --data-urlencode 'target=http://your-device-ip'
```

## Pull Request Process

1. Ensure all tests pass
2. Update the CHANGELOG.md with your changes
3. Update documentation if needed
4. Create a pull request with a clear description

## Release Process

Releases are automated via GitHub Actions when a new tag is pushed:

```bash
git tag v0.1.1
git push origin v0.1.1
```

This will:
- Create a GitHub release
- Build and push multi-arch Docker images
- Upload binary artifacts