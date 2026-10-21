// Modified by PastureStack in 2026: neutral, privacy-reduced client entrypoint.
package main

import (
	"os"

	"github.com/PastureStack/usage-telemetry-agent/internal/agent"
)

var (
	version = "dev"
	commit  = "unknown"
)

func main() {
	os.Exit(agent.Main(os.Args, version, commit, os.Stdout, os.Stderr))
}
