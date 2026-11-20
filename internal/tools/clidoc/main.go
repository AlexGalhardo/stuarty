// Command clidoc prints generated reference data as JSON for the documentation site (SDD-010 section 8):
// the commands, flags and exit codes of the binary, and with -networks the limits of every social network of
// the build, from the same data the product validates a post against.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/AlexGalhardo/stuarty/web/app/cli"
)

func main() {
	networks := flag.Bool("networks", false, "print the limits of the social networks instead of the CLI reference")
	flag.Parse()

	if err := run(os.Stdout, *networks); err != nil {
		fmt.Fprintln(os.Stderr, "clidoc:", err)
		os.Exit(1)
	}
}

func run(w io.Writer, networks bool) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)

	if networks {
		return enc.Encode(networkReference())
	}

	return enc.Encode(cli.Reference())
}
