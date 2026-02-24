package main

import (
	"os"

	"github.com/dorkyrobot/yelo/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
