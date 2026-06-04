package main

import (
	"bytes"
	"errors"
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
	acaName := strings.TrimSpace(os.Getenv("ACA_NAME"))
	acaImage := strings.TrimSpace(os.Getenv("ACA_IMAGE"))
	acaResourceGroup := strings.TrimSpace(os.Getenv("ACA_RESOURCE_GROUP"))

	if acaName == "" {
		return errors.New("ACA_NAME is required")
	}
	if acaImage == "" {
		return errors.New("ACA_IMAGE is required")
	}

	if acaResourceGroup == "" {
		var err error
		acaResourceGroup, err = discoverResourceGroup(acaName)
		if err != nil {
			return err
		}
	}

	fmt.Printf("Updating Container App %q in resource group %q to image %q\n", acaName, acaResourceGroup, acaImage)

	if _, err := runCommand("az", "containerapp", "update", "--name", acaName, "--resource-group", acaResourceGroup, "--image", acaImage, "--output", "none"); err != nil {
		return err
	}

	effectiveImage, err := runCommandSilent("az", "containerapp", "show", "--name", acaName, "--resource-group", acaResourceGroup, "--query", "properties.template.containers[0].image", "-o", "tsv")
	if err != nil {
		return err
	}

	effectiveImage = strings.TrimSpace(effectiveImage)
	if effectiveImage == "" || strings.EqualFold(effectiveImage, "none") {
		return errors.New("container app updated, but failed to read effective image")
	}

	fmt.Printf("Container App image is now: %s\n", effectiveImage)
	appendStepSummary(acaName, acaResourceGroup, acaImage, effectiveImage)

	return nil
}

func discoverResourceGroup(acaName string) (string, error) {
	out, err := runCommandSilent(
		"az",
		"resource",
		"list",
		"--name", acaName,
		"--resource-type", "Microsoft.App/containerApps",
		"--query", "[0].resourceGroup",
		"-o", "tsv",
	)
	if err != nil {
		return "", err
	}

	resourceGroup := strings.TrimSpace(out)
	if resourceGroup == "" || strings.EqualFold(resourceGroup, "none") {
		return "", fmt.Errorf("cannot find resource group for container app %q (set ACA_RESOURCE_GROUP explicitly)", acaName)
	}

	return resourceGroup, nil
}

func appendStepSummary(appName, rg, requestedImage, effectiveImage string) {
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

	_, _ = fmt.Fprintf(
		f,
		"\n## Container App Update\n\n- Container App: %s\n- Resource group: %s\n- Requested image: %s\n- Effective image: %s\n",
		appName,
		rg,
		requestedImage,
		effectiveImage,
	)
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