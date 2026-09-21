package main

import (
	"os"

	"openjev/features/jev-test/internal/cli"
	"openjev/features/jev-test/internal/logger"
)

func main() {
	log := logger.New(os.Stderr).WithComponent("jev-test")
	os.Exit(cli.Run(os.Args[1:], os.Stdout, os.Stderr, log))
}
