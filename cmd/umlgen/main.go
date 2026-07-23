package main

import (
	"fmt"
	"os"

	"github.com/Mino829/umlgen/internal/cli"
)

func main() {
	code, err := cli.Run(os.Args[1:], os.Stdout, os.Stderr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
	}
	os.Exit(code)
}
