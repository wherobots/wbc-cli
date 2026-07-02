package commands

import (
	"github.com/spf13/cobra"

	"wherobots/cli/internal/config"
)

// BuildBareRootCommand returns a root command shell without the
// spec-generated tree. main dispatches spec-free invocations (auth, upgrade)
// through it so they work with no credentials, no cached spec, and no
// network access to the API host.
func BuildBareRootCommand(cfg config.Config) *cobra.Command {
	return &cobra.Command{
		Use:           cfg.AppName,
		SilenceUsage:  true,
		SilenceErrors: true,
		CompletionOptions: cobra.CompletionOptions{
			DisableDefaultCmd: true,
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
}

// IsSpecFreeInvocation reports whether args resolve to a command registered
// on the bare root (auth, upgrade) rather than the spec-generated tree.
func IsSpecFreeInvocation(root *cobra.Command, args []string) bool {
	cmd, _, err := root.Find(args)
	if err != nil || cmd == nil || cmd == root {
		return false
	}
	for cmd.Parent() != nil && cmd.Parent() != root {
		cmd = cmd.Parent()
	}
	return cmd.Parent() == root
}
