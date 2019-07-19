package main

import (
	"os"

	"github.com/carlmjohnson/exitcode"
	"github.com/spotlightpa/linkrot/linkcheck"
)

func main() {
	exitcode.Exit(linkcheck.CLI(os.Args[1:]))
}
