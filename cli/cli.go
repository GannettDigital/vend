package cli

import (
	"flag"
	"fmt"
)

// Options contains CLI arguments passed to the program.
type Options struct {
	Help     bool
	Debug    bool
	Quiet    bool
	BasePath string
	Filter   string
}

// ParseOptions parses the command line options and returns a struct filled with
// the relevant options.
func ParseOptions() Options {
	var opt Options

	flag.BoolVar(&opt.Help, "help", false, "Show help.")
	flag.BoolVar(&opt.Debug, "debug", false, "Show debugging output.")
	flag.BoolVar(&opt.Quiet, "quiet", false, "Suppress normal output.")
	flag.StringVar(&opt.BasePath, "output", "vendor", "Directory to write vendored packages.")
	flag.StringVar(&opt.Filter, "filter", "", "Filter which files are written to directory.")
	flag.Parse()

	return opt
}

// PrintUsage prints the usage of this tool.
func (opt *Options) PrintUsage() {
	const banner string = `                     _
__   _____ _ __   __| |
\ \ / / _ \ '_ \ / _' |
 \ V /  __/ | | | (_| |
  \_/ \___|_| |_|\__,_|

`

	fmt.Println(banner)
	fmt.Printf("A small command line utility for fully vendoring module dependencies\n\n")

	flag.Usage()
}
