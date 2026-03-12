package commands

import (
	"fmt"

	"github.com/spf13/cobra"
)

// AddVersionCommand registers a "version" subcommand that prints the full
// version string. This complements the --version flag that cobra provides
// on the root command.
func AddVersionCommand(root *cobra.Command, versionString string) {
	root.AddCommand(&cobra.Command{
		Use:           "version",
		Short:         "Print the CLI version",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, err := fmt.Fprintln(cmd.OutOrStdout(), versionString)
			return err
		},
	})
}
