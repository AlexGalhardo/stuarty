// Command clidoc prints the CLI reference as JSON for the documentation site
// (SDD-010 section 8): the commands, flags and exit codes of the binary.
package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/AlexGalhardo/stuarty/web/app/cli"
)

func main() {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)

	if err := enc.Encode(cli.Reference()); err != nil {
		fmt.Fprintln(os.Stderr, "clidoc:", err)
		os.Exit(1)
	}
}
