package main

import (
	"fmt"
	"os"

	"github.com/dorkyrobot/yelo/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "yelo: %v\n", err)
		os.Exit(1)
	}
}
