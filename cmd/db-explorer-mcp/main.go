package main

import (
	"fmt"
	"os"

	"github.com/rogick/db-explorer-mcp/pkg/mcp"
)

func main() {
	srv := mcp.NewServer()
	if err := srv.ServeStdio(); err != nil {
		fmt.Fprintf(os.Stderr, "Erro fatal no servidor DB Explorer MCP: %v\n", err)
		os.Exit(1)
	}
}
