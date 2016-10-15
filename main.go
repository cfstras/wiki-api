package main

import (
	"flag"
	"os"

	"fmt"

	"github.com/cfstras/wiki-api/api"
)

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage of %s:\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "    %s <repository>\n", os.Args[0])
		flag.PrintDefaults()
	}

	var listenOn string
	var debug bool
	flag.StringVar(&listenOn, "l", ":3000", "Bind address")
	flag.BoolVar(&debug, "debug", false, "Enable /debug/pprof")

	flag.Parse()
	if len(flag.Args()) != 1 {
		flag.Usage()
		return
	}
	repoPath := flag.Args()[0]

	fmt.Println(api.Run(listenOn, repoPath, debug))
}
