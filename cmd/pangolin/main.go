package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/pangolin/pangolin/internal/app"
	"github.com/pangolin/pangolin/internal/version"
)

func main() {
	showVersion := flag.Bool("version", false, "print version")
	configPath := flag.String("config", "", "configuration file path")
	flag.Parse()
	if *showVersion {
		io.WriteString(os.Stdout, version.Version+"\n")
		return
	}
	if err := app.Run(*configPath); err != nil {
		io.WriteString(os.Stderr, "Pangolin failed to start: "+err.Error()+"\nPlease fix the configuration and try again.\n")
		waitForConfirmation()
		os.Exit(1)
	}
}

func waitForConfirmation() {
	info, err := os.Stdin.Stat()
	if err != nil || info.Mode()&os.ModeCharDevice == 0 {
		return
	}
	io.WriteString(os.Stderr, "Press Enter to exit.")
	_, _ = fmt.Fscanln(os.Stdin)
}
