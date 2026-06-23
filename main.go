package main

import (
	"github.com/junfuchang/superflare/cmd"
	"github.com/junfuchang/superflare/internal/server"
)

func main() {
	flags := cmd.Parse()
	server.StartDaemon(&flags)
}
