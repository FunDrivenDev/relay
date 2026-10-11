// Command relay runs Claude Code under tmux with the handover plugin's relay:
// relay <entity> <verb>, driven alike by people and agents (docs/contract.md).
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/FunDrivenDev/relay/internal/cli"
	"github.com/FunDrivenDev/relay/internal/commands"
)

// version is set at build time (-ldflags "-X main.version=…").
var version = "dev"

func main() {
	os.Exit(relay(commands.Entities()))
}

// relay runs the program over entities and returns its exit status; the
// functional tests run it with the test-only fixture entity added.
func relay(entities []*cli.Entity) int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return cli.NewApp("relay", version, entities).Main(ctx, os.Args[1:], cli.OS())
}
