package app

import "os/exec"

const ClaudeInstallCmd = "curl -fsSL https://claude.ai/install.sh | bash"

func ClaudeAvailable() bool {
	_, err := exec.LookPath("claude")
	return err == nil
}
