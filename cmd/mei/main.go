package main

import (
	"fmt"
	"os"

	"github.com/mindfire-test/meiosis/internal/cli"
)

func main() {
	if err := cli.New(nil).Execute(); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
