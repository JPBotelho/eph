package main

import (
	"fmt"
	"jcosta/ephemeral-prom/ingester"
	"jcosta/ephemeral-prom/querier"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: myexecutable <command> [args...]")
		os.Exit(1)
	}

	cmd := os.Args[1]

	switch cmd {
	case "querier":
		// Pass remaining args to querier
		querier.Run(os.Args[2:])
	case "ingester":
		ingester.Run(os.Args[2:])
	default:
		fmt.Printf("Unknown command: %s\n", cmd)
		os.Exit(1)
	}
}
