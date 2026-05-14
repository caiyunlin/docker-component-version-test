# docker-component-version-test

This is a small Go and Docker test project for printing component versions during
Docker builds and GitHub Actions runs.

## What it does

- Builds a Go application that prints a greeting and a generated UUID.
- Uses a multi-stage Dockerfile to build the Go binary once.
- Resolves selected Go modules with `go get module@latest` during Docker builds.
- Produces Alpine and Ubuntu based runtime images.
- Prints versions for Go modules, base images, and selected system components.
- Runs in GitHub Actions so the version output is visible in workflow logs.

## Local usage

Build and run the Alpine image:

```bash
docker build --progress=plain --target release-alpine --tag docker-component-version-test:alpine .
docker run --rm docker-component-version-test:alpine
```

Build and run the Ubuntu image:

```bash
docker build --progress=plain --target release-ubuntu --tag docker-component-version-test:ubuntu .
docker run --rm docker-component-version-test:ubuntu
```

Run the Go application directly:

```bash
go run .
```

## GitHub Actions

The workflow in `.github/workflows/component-versions.yml` can be run manually or
triggered by pushes to `main` or `master`. It prints:

- GitHub Actions runner, Docker, and Buildx versions.
- Go version and Go module dependency versions resolved during Docker build.
- Alpine version and selected Alpine package versions.
- Ubuntu version and selected Ubuntu package versions.
- Application output from both runtime images.
