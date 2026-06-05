package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

const (
	mediaTypeDockerManifestList = "application/vnd.docker.distribution.manifest.list.v2+json"
	mediaTypeOCIImageIndex      = "application/vnd.oci.image.index.v1+json"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	acrName := strings.TrimSpace(os.Getenv("ACR_NAME"))
	acrRepository := strings.TrimSpace(os.Getenv("ACR_REPOSITORY"))
	acrLoginServer := strings.TrimSpace(os.Getenv("ACR_LOGIN_SERVER"))
	currentVersion := strings.TrimSpace(os.Getenv("CURRENT_VERSION"))
	nextVersion := strings.TrimSpace(os.Getenv("NEXT_VERSION"))
	currentDigest := strings.TrimSpace(os.Getenv("CURRENT_DIGEST"))
	nextDigest := strings.TrimSpace(os.Getenv("NEXT_DIGEST"))

	if acrName == "" {
		return fmt.Errorf("ACR_NAME is required")
	}
	if acrRepository == "" {
		return fmt.Errorf("ACR_REPOSITORY is required")
	}
	if nextDigest == "" {
		return fmt.Errorf("NEXT_DIGEST is required")
	}

	currentImageSize, _ := resolveEffectiveSize(acrName, acrRepository, currentDigest)
	nextImageSize, _ := resolveEffectiveSize(acrName, acrRepository, nextDigest)

	if currentImageSize != "" {
		setGitHubEnv("CURRENT_IMAGE_SIZE", currentImageSize)
	} else {
		setGitHubEnv("CURRENT_IMAGE_SIZE", "")
	}

	if nextImageSize != "" {
		setGitHubEnv("NEXT_IMAGE_SIZE", nextImageSize)
	} else {
		setGitHubEnv("NEXT_IMAGE_SIZE", "")
	}

	result := "digest/size indicate content change"
	shouldRunTerratest := "false"

	if currentDigest != "" && currentDigest == nextDigest {
		result = "current and next digest are identical"
		shouldRunTerratest = "false"
	} else if currentDigest != "" && currentImageSize != "" && nextImageSize != "" && currentImageSize == nextImageSize {
		result = "digest changed but effective image size unchanged"
		shouldRunTerratest = "false"
	} else if currentDigest != "" && currentImageSize != "" && nextImageSize != "" && currentDigest != nextDigest && currentImageSize != nextImageSize {
		result = "digest and effective image size both changed (run terratest)"
		shouldRunTerratest = "true"
	}

	setGitHubEnv("SHOULD_RUN_TERRATEST", shouldRunTerratest)
	appendStepSummary(acrLoginServer, acrRepository, currentVersion, nextVersion, currentDigest, nextDigest, currentImageSize, nextImageSize, result)

	fmt.Printf("Result: %s\n", result)
	fmt.Printf("SHOULD_RUN_TERRATEST=%s\n", shouldRunTerratest)

	return nil
}

func resolveEffectiveSize(acrName, repository, digest string) (string, error) {
	if digest == "" {
		return "", nil
	}

	mediaType, err := queryTSV(
		"az", "acr", "manifest", "list-metadata",
		"--registry", acrName,
		"--name", repository,
		"--orderby", "time_desc",
		"--query", fmt.Sprintf("[?digest=='%s'].mediaType | [0]", digest),
		"-o", "tsv",
	)
	if err != nil {
		return "", nil
	}

	if mediaType == mediaTypeDockerManifestList || mediaType == mediaTypeOCIImageIndex {
		childDigest, _ := queryTSV(
			"az", "acr", "manifest", "show",
			"--registry", acrName,
			"--name", fmt.Sprintf("%s@%s", repository, digest),
			"--query", "manifests[?platform.os=='linux' && platform.architecture=='amd64'].digest | [0]",
			"-o", "tsv",
		)

		if childDigest == "" || strings.EqualFold(childDigest, "None") {
			childDigest, _ = queryTSV(
				"az", "acr", "manifest", "show",
				"--registry", acrName,
				"--name", fmt.Sprintf("%s@%s", repository, digest),
				"--query", "manifests[0].digest",
				"-o", "tsv",
			)
		}

		if childDigest != "" && !strings.EqualFold(childDigest, "None") {
			childSize, _ := queryTSV(
				"az", "acr", "manifest", "list-metadata",
				"--registry", acrName,
				"--name", repository,
				"--orderby", "time_desc",
				"--query", fmt.Sprintf("[?digest=='%s'].imageSize | [0]", childDigest),
				"-o", "tsv",
			)
			if childSize != "" && !strings.EqualFold(childSize, "None") && childSize != "0" {
				return childSize, nil
			}
		}
	}

	directSize, err := queryTSV(
		"az", "acr", "manifest", "list-metadata",
		"--registry", acrName,
		"--name", repository,
		"--orderby", "time_desc",
		"--query", fmt.Sprintf("[?digest=='%s'].imageSize | [0]", digest),
		"-o", "tsv",
	)
	if err != nil {
		return "", nil
	}

	if directSize != "" && !strings.EqualFold(directSize, "None") && directSize != "0" {
		return directSize, nil
	}

	return "", nil
}

func appendStepSummary(acrLoginServer, repository, currentVersion, nextVersion, currentDigest, nextDigest, currentImageSize, nextImageSize, result string) {
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
	if currentImageSize == "" {
		currentImageSize = "<unknown>"
	}
	if nextImageSize == "" {
		nextImageSize = "<unknown>"
	}

	_, _ = fmt.Fprintf(
		f,
		"## Component Versions ACR Publish\n\n- Registry: %s\n- Repository: %s\n- Current version: %s\n- Next version: %s\n- Current digest: %s\n- Next digest: %s\n- Current image size: %s\n- Next image size: %s\n- Result: %s\n",
		acrLoginServer,
		repository,
		currentVersion,
		nextVersion,
		currentDigest,
		nextDigest,
		currentImageSize,
		nextImageSize,
		result,
	)
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

func queryTSV(name string, args ...string) (string, error) {
	out, err := runCommandSilent(name, args...)
	if err != nil {
		return "", err
	}

	val := strings.TrimSpace(out)
	if strings.EqualFold(val, "None") {
		return "", nil
	}

	return val, nil
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
