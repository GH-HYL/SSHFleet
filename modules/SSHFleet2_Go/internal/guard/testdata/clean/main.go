package main

import (
	"os"

	"fixture/internal/console"
	"fixture/internal/parse"
)

func main() {
	_ = console.Line
	_ = parse.Run
	os.Exit(0)
}
