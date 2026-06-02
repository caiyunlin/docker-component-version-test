package test

import (
	"os"
	"strings"
	"testing"

	"github.com/gruntwork-io/terratest/modules/docker"
	"github.com/stretchr/testify/assert"
)

func getImageTag() string {
	imageTag := strings.TrimSpace(os.Getenv("IMAGE_UNDER_TEST"))
	if imageTag == "" {
		return "docker-component-version-test:ubuntu"
	}

	return imageTag
}

func TestDockerImageTools(t *testing.T) {
	imageTag := getImageTag()
	t.Logf("Testing image: %s", imageTag)

	tests := []struct {
		name    string
		command []string
		check   func(t *testing.T, output string)
	}{
		{
			name: "Required Ubuntu tooling packages installed",
			command: []string{
				"bash",
				"-lc",
				"set -euo pipefail; for pkg in apt-transport-https ca-certificates software-properties-common httping man-db vim screen curl gnupg atop htop jq dnsutils tcpdump traceroute iputils-ping iptables net-tools ncat iproute2 strace telnet openssl psmisc dsniff mtr-tiny conntrack ethtool iputils-tracepath lsof nmap socat sysstat wget; do dpkg -s \"$pkg\" >/dev/null; done; echo all-tools-installed",
			},
			check: func(t *testing.T, output string) {
				assert.Contains(t, output, "all-tools-installed", "Some required Ubuntu tools are missing")
			},
		},
		{
			name:    "Main app binary starts",
			command: []string{"./main"},
			check: func(t *testing.T, output string) {
				assert.Contains(t, output, "Hello from docker-component-version-test!", "Application binary output mismatch")
			},
		},
		{
			name:    "OS is Ubuntu",
			command: []string{"bash", "-lc", "cat /etc/os-release"},
			check: func(t *testing.T, output string) {
				assert.Contains(t, output, "Ubuntu", "Image is not Ubuntu-based")
			},
		},
	}

	for _, tc := range tests {
		tc := tc // capture range variable
		t.Run(tc.name, func(t *testing.T) {
			t.Logf("Running command: %v", tc.command)
			opts := &docker.RunOptions{
				Command:  tc.command,
				Platform: "linux/amd64",
				OtherOptions: []string{"--entrypoint", ""},
			}
			output := docker.Run(t, imageTag, opts)
			t.Logf("Command output:\n%s", output)
			tc.check(t, output)
		})
	}
}
