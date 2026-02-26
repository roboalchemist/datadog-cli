package main

import (
	"embed"
	"io/fs"

	"github.com/roboalchemist/datadog-cli/cmd"
)

// version is set via ldflags at build time: -X main.version=<version>
var version = "dev"

//go:embed README.md
var readmeContents string

//go:embed skill/SKILL.md
var skillMD string

//go:embed skill/reference/commands.md
var commandsRef string

//go:embed skill
var skillFS embed.FS

func main() {
	cmd.SetVersion(version)
	cmd.SetReadmeContents(readmeContents)
	cmd.SetSkillData(skillMD, commandsRef, fs.FS(skillFS))
	cmd.Execute()
}
