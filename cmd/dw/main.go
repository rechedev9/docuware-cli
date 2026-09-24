// Command dw is a DocuWare command line client.
package main

import (
	"runtime/debug"

	"github.com/rechedev9/docuware-cli/internal/cli"
)

// version is set by release builds with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	if version == "dev" {
		// go install records the module version in the build info.
		if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
			version = info.Main.Version
		}
	}
	cli.Main(version)
}
