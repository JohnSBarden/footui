package cli

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/JohnSBarden/footui/internal/config"
)

// skipSettings marks a command that must run without resolving settings,
// so a broken settings file can be inspected or fixed.
const skipSettings = "footui/skip-settings"

func newConfigCmd(g *globalFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Show, create, or locate the settings file",
		Long: "footui reads optional settings from a YAML file at " +
			"$XDG_CONFIG_HOME/footui/config.yaml (or $FOOTUI_CONFIG). A command-line flag " +
			"always wins over an environment variable, which wins over the file.\n\n" +
			"With no subcommand, config shows the settings in effect and where each came from.",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return showSettings(cmd, g)
		},
	}

	cmd.AddCommand(&cobra.Command{
		Use:         "path",
		Short:       "Print where the settings file is read from",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{skipSettings: "1"},
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintln(cmd.OutOrStdout(), configPath())
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:          "init",
		Short:        "Write a commented settings file with every option at its default",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		Annotations:  map[string]string{skipSettings: "1"},
		RunE: func(cmd *cobra.Command, args []string) error {
			path := configPath()
			if err := config.WriteTemplate(path); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Wrote %s — uncomment a line to change it.\n", path)
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:          "show",
		Short:        "Show the settings in effect and where each came from",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return showSettings(cmd, g)
		},
	})

	return cmd
}

func showSettings(cmd *cobra.Command, g *globalFlags) error {
	s := g.settings
	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "Settings file: %s\n\n", configPath())
	rows := []struct{ name, value string }{
		{"theme", s.theme.String()},
		{"accent", s.accent.String()},
		{"stale_after", staleText(s)},
		{"db", s.dbPath},
		{"no_color", strconv.FormatBool(s.noColor)},
		{"debug", debugText(s)},
	}
	for _, r := range rows {
		fmt.Fprintf(out, "  %-12s %-40s (%s)\n", r.name, r.value, s.sources[r.name])
	}
	return nil
}

func debugText(s settings) string {
	if s.debug {
		return "on → " + config.DebugLogPath()
	}
	return "off"
}

func staleText(s settings) string {
	if s.staleAfter <= 0 {
		return "off"
	}
	return s.staleAfter.String()
}
