package cmd

import (
	"fmt"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// walkCommands visits c and every command beneath it.
func walkCommands(c *cobra.Command, fn func(*cobra.Command)) {
	fn(c)
	for _, sub := range c.Commands() {
		walkCommands(sub, fn)
	}
}

// TestNoFlagShorthandCollisions guards against a persistent flag on a parent
// command claiming a shorthand that a subcommand's local flag already uses.
//
// Cobra registers local and persistent flags in separate FlagSets, so such a
// collision is invisible at init time. It only surfaces when cobra merges the
// two sets, which happens the first time the subcommand actually runs -- at
// which point pflag panics and the command is unusable. That is how a global
// `--json`/`-j` broke every `apppack db load` invocation for six weeks
// (4.7.0 through 4.8.1).
//
// Calling Flags(), LocalFlags() and InheritedFlags() on each command forces
// that merge, so the collision fails here instead of in a user's terminal.
func TestNoFlagShorthandCollisions(t *testing.T) {
	var collisions []string

	walkCommands(rootCmd, func(c *cobra.Command) {
		defer func() {
			if r := recover(); r != nil {
				collisions = append(collisions, fmt.Sprintf("%s: %v", c.CommandPath(), r))
			}
		}()

		noop := func(*pflag.Flag) {}
		c.Flags().VisitAll(noop)
		c.LocalFlags().VisitAll(noop)
		c.InheritedFlags().VisitAll(noop)
	})

	for _, c := range collisions {
		t.Errorf("flag shorthand collision: %s", c)
	}
}

// TestShorthandsUniquePerCommand asserts the same invariant declaratively, so
// it still holds if a future pflag release downgrades the collision from a
// panic to silent shadowing.
func TestShorthandsUniquePerCommand(t *testing.T) {
	walkCommands(rootCmd, func(c *cobra.Command) {
		defer func() { _ = recover() }() // collisions are reported by the test above

		byShorthand := map[string]string{}
		c.Flags().VisitAll(func(f *pflag.Flag) {
			if f.Shorthand == "" {
				return
			}
			if existing, taken := byShorthand[f.Shorthand]; taken && existing != f.Name {
				t.Errorf(
					"%s: shorthand -%s is claimed by both --%s and --%s",
					c.CommandPath(), f.Shorthand, existing, f.Name,
				)
			}
			byShorthand[f.Shorthand] = f.Name
		})
	})
}
