// Package commands is relay's registry: the one place an entity is wired in.
package commands

import (
	"github.com/FunDrivenDev/relay/internal/cli"
	"github.com/FunDrivenDev/relay/internal/doctor"
	"github.com/FunDrivenDev/relay/internal/session"
)

// Entities lists relay's entities: one line each, sorted by name.
func Entities() []*cli.Entity {
	return []*cli.Entity{
		doctor.Entity,
		session.Entity,
	}
}
