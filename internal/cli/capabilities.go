package cli

import (
	"errors"
	"flag"
	"io"

	"plenilune/internal/capabilities"
)

// runCapabilities serves the capabilities document (P2-2). Like
// `version`, the command is JSON-only: the document itself is the
// contract, so there is no second human-readable rendering to keep in
// sync. --json is accepted explicitly because callers are told to ask
// for it; omitting it prints the same document.
func runCapabilities(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("capabilities", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.Bool("json", false, "emit the capabilities document as JSON (the only form)")
	fs.Usage = func() { fs.SetOutput(stdout); fs.PrintDefaults() }
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fs.Usage()
			return 0
		}
		return fail(stderr, "USAGE", "invalid arguments; use --help")
	}
	if fs.NArg() != 0 {
		return fail(stderr, "USAGE", "unexpected positional argument")
	}
	return output(stdout, stderr, capabilities.Build())
}
