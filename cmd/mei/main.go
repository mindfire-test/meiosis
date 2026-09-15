package main

import (
	"fmt"
	"os"

	"github.com/mindfire-test/meiosis/internal/cli"
)

// version is stamped at build time: -ldflags "-X main.version=v1.2.3".
var version = "dev"

func main() {
	cli.Version = version
	if err := cli.New(nil).Execute(); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
