package main

import (
	"fmt"
	"os"

	"github.com/entelecheia/rootfiles-v2/internal/cli"
)

var (
	version = "dev"
	commit  = "none"
)

func main() {
	if err := cli.Execute(version, commit); err != nil {
		// Surface the error before exiting so upgrade/apply failures are
		// not silent. Historically `rootfiles upgrade` would exit 1 with no
		// output when the binary replace or a network call failed, making
		// the root cause invisible in CI logs.
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}
