package main

import (
	"os"

	"github.com/spf13/pflag"
	"github.com/vladimirvivien/ktop/cmd"
)

func main() {
	flags := pflag.NewFlagSet("ktop", pflag.ExitOnError)
	pflag.CommandLine = flags

	// run standalone
	if err := cmd.NewKtopCmd().Execute(); err != nil {
		os.Exit(1)
	}
}
