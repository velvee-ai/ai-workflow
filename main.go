package main

import (
	"github.com/velvee-ai/ai-workflow/cmd"
)

// Version information set by GoReleaser at build time
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	cmd.SetVersionInfo(version, commit, date)
	cmd.Execute()
}
