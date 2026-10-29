package main

import (
	"os"

	"github.com/dynatrace-oss/dtmgd/cmd"
)

func main() {
	os.Exit(cmd.Run(os.Args[1:], os.Stdout, os.Stderr))
}
