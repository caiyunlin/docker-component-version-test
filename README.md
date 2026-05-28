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

## Push to ACR only when digest changed

The Go tool below compares the current ACR tag digest with a newly built
candidate digest:

- If digest is the same: print a message and exit.

- If digest is different: run tests, then publish.

Notes:

- The script uses a temporary candidate tag (`candidate-<timestamp>`) for digest comparison.
- Promotion to final tag is done by digest (`docker buildx imagetools create`) to avoid rebuilding.
- Requires: Azure CLI (`az`) login, Docker Buildx, and access to the target ACR.

## Go implementation for conditional ACR push

You can run this locally or in CI:

- Go entry point: `cmd/acr-push-if-changed/main.go`
- Workflow integration: `.github/workflows/component-versions.yml`

Local example:

```bash
go run ./cmd/acr-push-if-changed \
	-acr-name <acr-name> \
	-repository docker-component-version-test \
	-tag 1.0 \
	-target release-ubuntu \
	-platform linux/amd64 \
	-test-cmd "go test ./..."
```

GitHub Actions setup:

- Configure repository secret `AZURE_CREDENTIALS` for `azure/login`.
- Configure repository secret `ACR_NAME` (for example: `myregistry`).
- Run workflow `Component Versions` from Actions tab.

The workflow runs the Go script and follows the same behavior:

- unchanged digest: prints and exits successfully.
- changed digest: runs test command and publishes the new digest.

## GitHub Actions

The workflow in `.github/workflows/component-versions.yml` can be run manually or
triggered by pushes to `main` or `master`. It prints:

- GitHub Actions runner, Docker, and Buildx versions.
- Go version and Go module dependency versions resolved during Docker build.
- Alpine version and selected Alpine package versions.
- Ubuntu version and selected Ubuntu package versions.
- Application output from both runtime images.
