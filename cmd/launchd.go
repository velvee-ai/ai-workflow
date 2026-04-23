package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

const launchdLabel = "ai.velvee.work-sync"

func launchdPlistPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Library", "LaunchAgents", launchdLabel+".plist"), nil
}

func launchdLogPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".work", "logs", "sync.log"), nil
}

func launchdUserDomain() string {
	return "gui/" + strconv.Itoa(os.Getuid())
}

// launchdLoaded reports whether the LaunchAgent is currently loaded.
func launchdLoaded() bool {
	out, _ := exec.Command("launchctl", "list", launchdLabel).Output()
	return len(out) > 0
}

// launchdEnsure writes the plist pointing at the current work binary, then
// loads it if not already loaded. If the plist changed, it reloads. Idempotent.
func launchdEnsure() error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolving work binary path: %w", err)
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}

	plistPath, err := launchdPlistPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(plistPath), 0755); err != nil {
		return fmt.Errorf("creating LaunchAgents dir: %w", err)
	}

	logPath, err := launchdLogPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(logPath), 0755); err != nil {
		return fmt.Errorf("creating log dir: %w", err)
	}

	plist := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>%s</string>
    <key>ProgramArguments</key>
    <array>
        <string>%s</string>
        <string>sync</string>
        <string>webhook</string>
        <string>listen</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>ThrottleInterval</key>
    <integer>10</integer>
    <key>StandardOutPath</key>
    <string>%s</string>
    <key>StandardErrorPath</key>
    <string>%s</string>
</dict>
</plist>
`, launchdLabel, exe, logPath, logPath)

	existing, _ := os.ReadFile(plistPath)
	if string(existing) != plist {
		if launchdLoaded() {
			_ = launchdBootout()
		}
		if err := os.WriteFile(plistPath, []byte(plist), 0644); err != nil {
			return fmt.Errorf("writing plist: %w", err)
		}
	}

	if !launchdLoaded() {
		if err := launchdBootstrap(plistPath); err != nil {
			return err
		}
	}
	return nil
}

func launchdBootstrap(plistPath string) error {
	out, err := exec.Command("launchctl", "bootstrap", launchdUserDomain(), plistPath).CombinedOutput()
	if err != nil {
		return fmt.Errorf("launchctl bootstrap failed: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func launchdBootout() error {
	target := launchdUserDomain() + "/" + launchdLabel
	out, err := exec.Command("launchctl", "bootout", target).CombinedOutput()
	if err != nil {
		msg := string(out)
		// Not-loaded is not an error for our purposes.
		if strings.Contains(msg, "Could not find") || strings.Contains(msg, "No such") {
			return nil
		}
		return fmt.Errorf("launchctl bootout failed: %w: %s", err, strings.TrimSpace(msg))
	}
	return nil
}
