package main

import (
	"github.com/GannettDigital/vend/cli"
	"github.com/GannettDigital/vend/file"
)

func main() {

	options := cli.ParseOptions()

	if options.Help {
		options.PrintUsage()

	} else {
		cli.UpdateModule(options)

		dir := file.InitVendorDir(options)
		dir.CopyDependencies()
	}
}
