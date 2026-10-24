package main

import (
	_ "embed"
	"fmt"
	"log"
	"os"
	"runtime"

	"icon-svg/internal/app"
)

//go:embed VERSION
var version string

func main() {
	if err := app.Run(version); err != nil {
		log.Print(err)
		if runtime.GOOS == "windows" {
			fmt.Print("按 Enter 退出…")
			fmt.Scanln()
		}
		os.Exit(1)
	}
}
