package main

import (
	"runtime/debug"

	"github.com/harnyk/tran/cmd"
)

// Set by goreleaser via -ldflags "-X main.version=... -X main.commit=... -X main.date=...".
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	if commit == "none" {
		// Local builds (go build / go install .) carry VCS info in build metadata.
		if info, ok := debug.ReadBuildInfo(); ok {
			for _, s := range info.Settings {
				switch s.Key {
				case "vcs.revision":
					commit = s.Value
					if len(commit) > 7 {
						commit = commit[:7]
					}
				case "vcs.time":
					date = s.Value
				case "vcs.modified":
					if s.Value == "true" {
						commit += "-dirty"
					}
				}
			}
		}
	}
	cmd.SetVersion(version, commit, date)
	cmd.Execute()
}
