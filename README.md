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

## Push to ACR only when image content changed

The workflow now compares the local build result with the latest ACR release image
before any push:

- Build the Ubuntu release image locally.
- Pull the current ACR version tag (latest semantic version) into the job.
- Run Go command `cmd/compare-acr-images` to compare local image digest (`docker image inspect .Id`) and image size (`.Size`) with the pulled image.
- If digest is the same: skip push.
- If digest is different but size is the same: skip push.
- Only when digest and size both indicate changes: push `NEXT_VERSION` to ACR.

This avoids creating unnecessary ACR versions when content is effectively unchanged.

## Go implementation for conditional ACR push

You can run this locally when you want a standalone command:

- Go entry point: `cmd/acr-push-if-changed/main.go`

Note:

- CI workflow uses `cmd/compare-acr-images` as the comparison and summary engine.
- `cmd/acr-push-if-changed` remains a standalone local/manual command.

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

The workflow follows this behavior:

- unchanged digest: skip publish and skip ACA update.
- changed digest + unchanged size: skip publish and skip ACA update.
- changed digest + changed size: publish new image, then continue compare/update steps.

## GitHub Actions

The workflow in `.github/workflows/component-versions.yml` can be run manually or
triggered by pushes to `main` or `master`. It prints:

- GitHub Actions runner, Docker, and Buildx versions.
- Go version and Go module dependency versions resolved during Docker build.
- Alpine version and selected Alpine package versions.
- Ubuntu version and selected Ubuntu package versions.
- Application output from both runtime images.
