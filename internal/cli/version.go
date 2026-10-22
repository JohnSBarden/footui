package cli

import (
	"fmt"
	"runtime/debug"

	"github.com/spf13/cobra"
)

// `go install …@v1.0.0` skips the Makefile's -ldflags, so fall back to the
// module version Go records in the binary.
func init() {
	if Version != "dev" {
		return
	}
	if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
		Version = bi.Main.Version
	}
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the footui version",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintln(cmd.OutOrStdout(), Version)
			return nil
		},
	}
}
