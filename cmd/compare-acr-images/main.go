package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	acrRepository := strings.TrimSpace(os.Getenv("ACR_REPOSITORY"))
	acrLoginServer := strings.TrimSpace(os.Getenv("ACR_LOGIN_SERVER"))
	currentVersion := strings.TrimSpace(os.Getenv("CURRENT_VERSION"))
	nextVersion := strings.TrimSpace(os.Getenv("NEXT_VERSION"))
	currentDigest := strings.TrimSpace(os.Getenv("CURRENT_DIGEST"))
	localImage := strings.TrimSpace(os.Getenv("LOCAL_IMAGE"))

	if acrLoginServer == "" {
		return fmt.Errorf("ACR_LOGIN_SERVER is required")
	}
	if acrRepository == "" {
		return fmt.Errorf("ACR_REPOSITORY is required")
	}
	if localImage == "" {
		localImage = "docker-component-version-test:ubuntu"
	}

	currentImage := ""
	if currentVersion != "" {
		currentImage = fmt.Sprintf("%s/%s:%s", acrLoginServer, acrRepository, currentVersion)
	}

	localImageID, err := dockerInspect(localImage, "{{.Id}}")
	if err != nil {
		return fmt.Errorf("cannot inspect local image %q: %w", localImage, err)
	}
	localImageSize, err := dockerInspect(localImage, "{{.Size}}")
	if err != nil {
		return fmt.Errorf("cannot inspect local image size %q: %w", localImage, err)
	}
	localRootFSLayers, err := dockerInspect(localImage, "{{json .RootFS.Layers}}")
	if err != nil {
		return fmt.Errorf("cannot inspect local image RootFS layers %q: %w", localImage, err)
	}

	setGitHubEnv("LOCAL_IMAGE_ID", localImageID)
	setGitHubEnv("LOCAL_IMAGE_SIZE", localImageSize)
	setGitHubEnv("LOCAL_ROOTFS_LAYERS", localRootFSLayers)

	currentImageID := ""
	currentImageSize := ""
	currentRootFSLayers := ""
	currentImageRepoDigest := ""

	shouldPublish := "true"
	shouldRunTerratest := "true"
	result := "No current ACR image found; publish required"

	if currentImage != "" {
		if _, pullErr := runCommand("docker", "pull", currentImage); pullErr == nil {
			currentImageID, _ = dockerInspect(currentImage, "{{.Id}}")
			currentImageSize, _ = dockerInspect(currentImage, "{{.Size}}")
			currentRootFSLayers, _ = dockerInspect(currentImage, "{{json .RootFS.Layers}}")
			currentImageRepoDigest, _ = dockerInspect(currentImage, "{{index .RepoDigests 0}}")

			if localImageID != "" && localImageID == currentImageID {
				shouldPublish = "false"
				shouldRunTerratest = "false"
				result = "Local image digest equals current ACR image digest"
			} else if localRootFSLayers != "" && localRootFSLayers == currentRootFSLayers {
				shouldPublish = "false"
				shouldRunTerratest = "false"
				result = "Digest differs but RootFS layers equal current ACR image"
			} else if localImageSize != "" && localImageSize == currentImageSize {
				shouldPublish = "false"
				shouldRunTerratest = "false"
				result = "Digest differs but image size equals current ACR image"
			} else {
				result = "Digest, RootFS layers, and size indicate changes; publish required"
			}
		} else {
			result = "Cannot pull current ACR image; publish required"
		}
	}

	setGitHubEnv("CURRENT_IMAGE_ID", currentImageID)
	setGitHubEnv("CURRENT_IMAGE_SIZE", currentImageSize)
	setGitHubEnv("CURRENT_ROOTFS_LAYERS", currentRootFSLayers)
	setGitHubEnv("CURRENT_IMAGE_REPO_DIGEST", currentImageRepoDigest)
	setGitHubEnv("SHOULD_PUBLISH", shouldPublish)
	setGitHubEnv("SHOULD_RUN_TERRATEST", shouldRunTerratest)
	setGitHubEnv("COMPARE_RESULT", result)

	nextDigest := strings.TrimSpace(os.Getenv("NEXT_DIGEST"))
	if shouldPublish != "true" {
		nextDigest = ""
	}

	appendStepSummary(acrLoginServer, acrRepository, currentVersion, nextVersion, currentDigest, nextDigest, currentImageID, localImageID, currentImageSize, localImageSize, currentRootFSLayers, localRootFSLayers, shouldPublish, shouldRunTerratest, result)

	fmt.Printf("Local image digest (Id): %s\n", fallback(localImageID, "<none>"))
	fmt.Printf("Local image size: %s\n", fallback(localImageSize, "<unknown>"))
	fmt.Printf("Local RootFS layers: %s\n", fallback(localRootFSLayers, "<unknown>"))
	if currentImageID != "" {
		fmt.Printf("Current ACR image digest (Id): %s\n", currentImageID)
		fmt.Printf("Current ACR image repo digest: %s\n", fallback(currentImageRepoDigest, "<none>"))
		fmt.Printf("Current ACR image size: %s\n", fallback(currentImageSize, "<unknown>"))
		fmt.Printf("Current ACR RootFS layers: %s\n", fallback(currentRootFSLayers, "<unknown>"))
	}
	fmt.Printf("Compare result: %s\n", result)
	fmt.Printf("SHOULD_PUBLISH=%s\n", shouldPublish)
	fmt.Printf("SHOULD_RUN_TERRATEST=%s\n", shouldRunTerratest)

	return nil
}

func appendStepSummary(acrLoginServer, repository, currentVersion, nextVersion, currentDigest, nextDigest, currentImageID, localImageID, currentImageSize, localImageSize, currentRootFSLayers, localRootFSLayers, shouldPublish, shouldRunTerratest, result string) {
	summaryPath := strings.TrimSpace(os.Getenv("GITHUB_STEP_SUMMARY"))
	if summaryPath == "" {
		return
	}

	f, err := os.OpenFile(summaryPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: cannot append step summary: %v\n", err)
		return
	}
	defer f.Close()

	if currentVersion == "" {
		currentVersion = "<none>"
	}
	if currentDigest == "" {
		currentDigest = "<none>"
	}
	if nextDigest == "" {
		nextDigest = "<none>"
	}
	if currentImageID == "" {
		currentImageID = "<none>"
	}
	if localImageID == "" {
		localImageID = "<none>"
	}
	if currentImageSize == "" {
		currentImageSize = "<unknown>"
	}
	if localImageSize == "" {
		localImageSize = "<unknown>"
	}
	if currentRootFSLayers == "" {
		currentRootFSLayers = "<unknown>"
	}
	if localRootFSLayers == "" {
		localRootFSLayers = "<unknown>"
	}

	_, _ = fmt.Fprintf(
		f,
		"## ACR Image Comparison\n\n- Repository: %s/%s\n- Current version: %s\n- Next version: %s\n- Compare result: %s\n- Should publish: %s\n- Should run terratest: %s\n\n| Item | Current ACR | Local Build / Next |\n| --- | --- | --- |\n| Tag | %s | %s |\n| ACR manifest digest | %s | %s |\n| Docker image digest (Id) | %s | %s |\n| Docker image size (bytes) | %s | %s |\n| RootFS layers | `%s` | `%s` |\n",
		acrLoginServer,
		repository,
		currentVersion,
		nextVersion,
		result,
		shouldPublish,
		shouldRunTerratest,
		currentVersion,
		nextVersion,
		currentDigest,
		nextDigest,
		currentImageID,
		localImageID,
		currentImageSize,
		localImageSize,
		currentRootFSLayers,
		localRootFSLayers,
	)
}

func fallback(value, fallbackValue string) string {
	if strings.TrimSpace(value) == "" {
		return fallbackValue
	}
	return value
}

func setGitHubEnv(key, value string) {
	githubEnv := strings.TrimSpace(os.Getenv("GITHUB_ENV"))
	if githubEnv == "" {
		return
	}

	f, err := os.OpenFile(githubEnv, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: cannot write GITHUB_ENV %s: %v\n", key, err)
		return
	}
	defer f.Close()

	_, _ = fmt.Fprintf(f, "%s=%s\n", key, value)
}

func dockerInspect(imageRef, format string) (string, error) {
	out, err := runCommandSilent("docker", "image", "inspect", imageRef, "--format", format)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

func runCommand(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	out := stdout.String()
	if out != "" {
		fmt.Print(out)
	}
	if err != nil {
		return out, fmt.Errorf("command failed: %s %s\n%s", name, strings.Join(args, " "), strings.TrimSpace(stderr.String()))
	}

	return out, nil
}

func runCommandSilent(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("command failed: %s %s\n%s", name, strings.Join(args, " "), strings.TrimSpace(stderr.String()))
	}

	return stdout.String(), nil
}
