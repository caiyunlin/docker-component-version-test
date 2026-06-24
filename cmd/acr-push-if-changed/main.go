package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
)

type manifest struct {
	Digest string   `json:"digest"`
	Tags   []string `json:"tags"`
}

type config struct {
	acrName            string
	repository         string
	tag                string
	target             string
	platform           string
	buildContext       string
	testCmd            string
	candidateTagPrefix string
}

func main() {
	cfg := parseFlags()

	if err := run(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func parseFlags() config {
	var cfg config

	flag.StringVar(&cfg.acrName, "acr-name", "", "ACR name (required)")
	flag.StringVar(&cfg.repository, "repository", "", "Image repository name (required)")
	flag.StringVar(&cfg.tag, "tag", "1.0", "Release tag to compare and publish")
	flag.StringVar(&cfg.target, "target", "release-ubuntu", "Docker build target stage")
	flag.StringVar(&cfg.platform, "platform", "linux/amd64", "Docker build platform")
	flag.StringVar(&cfg.buildContext, "build-context", ".", "Docker build context")
	flag.StringVar(&cfg.testCmd, "test-cmd", "go test ./...", "Test command executed when digest changed")
	flag.StringVar(&cfg.candidateTagPrefix, "candidate-tag-prefix", "candidate", "Temporary candidate tag prefix")
	flag.Parse()

	if cfg.acrName == "" {
		exitUsage("-acr-name is required")
	}
	if cfg.repository == "" {
		exitUsage("-repository is required")
	}

	return cfg
}

func exitUsage(msg string) {
	fmt.Fprintln(os.Stderr, msg)
	flag.Usage()
	os.Exit(2)
}

func run(cfg config) error {
	normalizedAcrName, err := normalizeAcrName(cfg.acrName)
	if err != nil {
		return err
	}

	fmt.Println("=== Login ACR ===")
	if _, err := runCommand("az", "acr", "login", "--name", normalizedAcrName); err != nil {
		return err
	}

	loginServer := fmt.Sprintf("%s.azurecr.io", normalizedAcrName)

	imageRef := fmt.Sprintf("%s/%s:%s", loginServer, cfg.repository, cfg.tag)
	candidateTag := fmt.Sprintf("%s-%d", cfg.candidateTagPrefix, time.Now().Unix())
	candidateRef := fmt.Sprintf("%s/%s:%s", loginServer, cfg.repository, candidateTag)

	fmt.Printf("=== Resolve existing digest for %s ===\n", imageRef)
	oldDigest, err := getTagDigest(normalizedAcrName, cfg.repository, cfg.tag)
	if err != nil {
		return err
	}
	if oldDigest == "" {
		fmt.Println("Existing digest: <none>")
	} else {
		fmt.Printf("Existing digest: %s\n", oldDigest)
	}

	fmt.Printf("=== Build and push candidate image: %s ===\n", candidateRef)
	if _, err := runCommand("docker", "buildx", "build", "--progress=plain", "--platform", cfg.platform, "--target", cfg.target, "--tag", candidateRef, "--push", cfg.buildContext); err != nil {
		return err
	}

	fmt.Printf("=== Resolve candidate digest for %s ===\n", candidateRef)
	newDigest, err := getTagDigest(normalizedAcrName, cfg.repository, candidateTag)
	if err != nil {
		return err
	}
	if newDigest == "" {
		return fmt.Errorf("cannot resolve candidate digest for %s", candidateRef)
	}
	fmt.Printf("Candidate digest: %s\n", newDigest)

	digestChanged := oldDigest != "" && newDigest != "" && oldDigest != newDigest
	if oldDigest != "" && !digestChanged {
		fmt.Println("Digest unchanged. Skip tests and publish.")
		cleanupCandidateTag(normalizedAcrName, cfg.repository, candidateTag)
		return nil
	}

	if oldDigest != "" {
		fmt.Println("Digest changed. Compare RootFS layers and image size.")

		oldRootFSLayers, oldRootFSKnown, oldRootFSErr := getImageRootFSLayers(imageRef)
		newRootFSLayers, newRootFSKnown, newRootFSErr := getImageRootFSLayers(candidateRef)

		if oldRootFSErr == nil && oldRootFSKnown {
			fmt.Printf("Existing RootFS layers: %s\n", oldRootFSLayers)
		} else if oldRootFSErr == nil {
			fmt.Println("Existing RootFS layers: <unknown>")
		} else {
			fmt.Fprintf(os.Stderr, "warning: cannot resolve existing RootFS layers: %v\n", oldRootFSErr)
		}
		if newRootFSErr == nil && newRootFSKnown {
			fmt.Printf("Candidate RootFS layers: %s\n", newRootFSLayers)
		} else if newRootFSErr == nil {
			fmt.Println("Candidate RootFS layers: <unknown>")
		} else {
			fmt.Fprintf(os.Stderr, "warning: cannot resolve candidate RootFS layers: %v\n", newRootFSErr)
		}

		oldSize, oldSizeKnown, oldSizeErr := getTagImageSize(normalizedAcrName, cfg.repository, cfg.tag)
		newSize, newSizeKnown, newSizeErr := getTagImageSize(normalizedAcrName, cfg.repository, candidateTag)

		if oldSizeErr == nil && oldSizeKnown {
			fmt.Printf("Existing image size: %d bytes\n", oldSize)
		} else if oldSizeErr == nil {
			fmt.Println("Existing image size: <unknown>")
		} else {
			fmt.Fprintf(os.Stderr, "warning: cannot resolve existing image size: %v\n", oldSizeErr)
		}
		if newSizeErr == nil && newSizeKnown {
			fmt.Printf("Candidate image size: %d bytes\n", newSize)
		} else if newSizeErr == nil {
			fmt.Println("Candidate image size: <unknown>")
		} else {
			fmt.Fprintf(os.Stderr, "warning: cannot resolve candidate image size: %v\n", newSizeErr)
		}

		rootFSLayersChanged := oldRootFSErr == nil && newRootFSErr == nil && oldRootFSKnown && newRootFSKnown && oldRootFSLayers != newRootFSLayers
		imageSizeChanged := oldSizeErr == nil && newSizeErr == nil && oldSizeKnown && newSizeKnown && oldSize != newSize

		if !rootFSLayersChanged {
			fmt.Println("Digest changed but RootFS layers are unchanged or unknown. Treat as unchanged and skip tests/publish.")
			cleanupCandidateTag(normalizedAcrName, cfg.repository, candidateTag)
			return nil
		}

		if !imageSizeChanged {
			fmt.Println("Digest and RootFS layers changed but image size is unchanged or unknown. Treat as unchanged and skip tests/publish.")
			cleanupCandidateTag(normalizedAcrName, cfg.repository, candidateTag)
			return nil
		}

		fmt.Println("Digest, RootFS layers, and image size all changed. Execute test phase.")
	} else {
		fmt.Println("No existing digest found. Execute test phase.")
	}

	if _, err := runShellCommand(cfg.testCmd); err != nil {
		return err
	}

	fmt.Printf("=== Promote candidate digest to release tag: %s ===\n", imageRef)
	if _, err := runCommand("docker", "buildx", "imagetools", "create", "--tag", imageRef, fmt.Sprintf("%s/%s@%s", loginServer, cfg.repository, newDigest)); err != nil {
		return err
	}

	cleanupCandidateTag(normalizedAcrName, cfg.repository, candidateTag)

	fmt.Println("Done. New digest published successfully.")
	return nil
}

func cleanupCandidateTag(acrName, repository, candidateTag string) {
	fmt.Println("=== Cleanup candidate tag (best effort) ===")
	if _, err := runCommand("az", "acr", "repository", "untag", "--name", acrName, "--image", fmt.Sprintf("%s:%s", repository, candidateTag)); err != nil {
		fmt.Fprintf(os.Stderr, "warning: cleanup failed for %s:%s: %v\n", repository, candidateTag, err)
	}
}

func normalizeAcrName(raw string) (string, error) {
	name := strings.TrimSpace(strings.ToLower(raw))
	name = strings.Trim(name, "\"'")
	name = strings.TrimPrefix(name, "https://")
	name = strings.TrimPrefix(name, "http://")
	name = strings.Split(name, "/")[0]
	name = strings.TrimSuffix(name, ".azurecr.io")
	name = strings.TrimSpace(name)

	if name == "" {
		return "", fmt.Errorf("invalid acr name: %q", raw)
	}

	return name, nil
}

func getTagDigest(acrName, repository, tag string) (string, error) {
	out, err := runCommandSilent("az", "acr", "repository", "show-manifests", "--name", acrName, "--repository", repository, "--orderby", "time_desc", "-o", "json")
	if err != nil {
		if strings.Contains(err.Error(), "NOT_FOUND") || strings.Contains(err.Error(), "repository") {
			return "", nil
		}
		return "", err
	}

	var manifests []manifest
	if err := json.Unmarshal([]byte(out), &manifests); err != nil {
		return "", err
	}

	for _, m := range manifests {
		for _, t := range m.Tags {
			if t == tag {
				return m.Digest, nil
			}
		}
	}

	return "", nil
}

func getTagImageSize(acrName, repository, tag string) (int64, bool, error) {
	out, err := runCommandSilent("az", "acr", "repository", "show", "--name", acrName, "--image", fmt.Sprintf("%s:%s", repository, tag), "--query", "imageSize", "-o", "tsv")
	if err != nil {
		if strings.Contains(err.Error(), "NOT_FOUND") || strings.Contains(err.Error(), "repository") || strings.Contains(err.Error(), "manifest") {
			return 0, false, nil
		}
		return 0, false, err
	}

	sizeRaw := strings.TrimSpace(out)
	if sizeRaw == "" {
		return 0, false, nil
	}

	size, parseErr := strconv.ParseInt(sizeRaw, 10, 64)
	if parseErr != nil {
		return 0, false, fmt.Errorf("invalid image size %q for %s:%s", sizeRaw, repository, tag)
	}

	return size, true, nil
}

func getImageRootFSLayers(imageRef string) (string, bool, error) {
	if _, err := runCommand("docker", "pull", imageRef); err != nil {
		return "", false, err
	}

	out, err := runCommandSilent("docker", "image", "inspect", imageRef, "--format", "{{json .RootFS.Layers}}")
	if err != nil {
		return "", false, err
	}

	layers := strings.TrimSpace(out)
	if layers == "" || layers == "null" {
		return "", false, nil
	}

	return layers, true, nil
}

func runShellCommand(command string) (string, error) {
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("powershell", "-NoProfile", "-Command", command)
	} else {
		cmd = exec.Command("bash", "-lc", command)
	}

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
		return out, fmt.Errorf("command failed (%s): %w\n%s", command, err, strings.TrimSpace(stderr.String()))
	}

	return out, nil
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
		return out, fmt.Errorf("command failed: %s %s: %w\n%s", name, strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}

	return out, nil
}

func runCommandSilent(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	out := stdout.String()
	if err != nil {
		return out, fmt.Errorf("command failed: %s %s: %w\n%s", name, strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}

	return out, nil
}
