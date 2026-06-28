package main

import (
	"log"
	"os"

	"github.com/junfuchang/superflare/cmd"
	"github.com/junfuchang/superflare/internal/server"
)

func main() {
	flags, err := cmd.ParseE()
	if err != nil {
		log.Printf("superflare startup config error: %v", err)
		os.Exit(1)
	}
	if err := server.StartDaemon(&flags); err != nil {
		log.Printf("superflare server exited with error: %v", err)
		os.Exit(1)
	}
}
