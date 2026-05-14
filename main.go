package main

import (
	"os"

	"github.com/pajarori/assetlas/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
