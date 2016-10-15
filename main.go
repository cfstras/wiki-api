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
	flag.StringVar(&listenOn, "l", ":3000", "Bind address")

	flag.Parse()
	if len(flag.Args()) != 1 {
		flag.Usage()
		return
	}
	repoPath := flag.Args()[0]

	fmt.Println(api.Run(listenOn, repoPath))
}
