package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	flag.Parse()
	fmt.Fprintln(os.Stderr, "agentctl: execution and ACP integration are not implemented")
	os.Exit(1)
}
