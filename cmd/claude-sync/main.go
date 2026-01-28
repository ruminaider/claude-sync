package main

import (
	"fmt"
	"os"
)

const version = "0.1.0-dev"

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(0)
	}

	command := os.Args[1]

	switch command {
	case "version", "--version", "-v":
		fmt.Printf("claude-sync %s\n", version)
	case "help", "--help", "-h":
		printUsage()
	case "status":
		fmt.Println("claude-sync status: not yet implemented")
	case "init":
		fmt.Println("claude-sync init: not yet implemented")
	case "pull":
		fmt.Println("claude-sync pull: not yet implemented")
	case "push":
		fmt.Println("claude-sync push: not yet implemented")
	default:
		fmt.Printf("Unknown command: %s\n", command)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println(`claude-sync - Sync Claude Code configuration across machines

USAGE:
    claude-sync [command]

COMMANDS:
    init                Create new config from current setup
    join <url>          Join existing config repo
    status              Show sync status
    pull                Pull latest config
    push                Push changes to remote
    update              Apply available plugin updates
    help                Show this help
    version             Show version

Run 'claude-sync help <command>' for more information on a command.`)
}
