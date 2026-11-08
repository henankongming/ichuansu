package main

import (
	"os"

	"github.com/henankongming/ichuansu/internal/app"
)

// version is set at build time with -ldflags. Local builds report "dev".
var version = "dev"

func main() {
	os.Exit(app.Run(os.Args[1:], version, os.Stdout, os.Stderr))
}
