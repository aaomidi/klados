package cmd

import (
	"embed"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// runDesktop is provided by desktop.go (default build) and desktop_stub.go
// (-tags headless), so server images build without CGO/GTK/Wails.
var runDesktop func(assets embed.FS) error

func Execute(assets embed.FS) {
	root := &cobra.Command{
		Use:           "klados",
		Short:         "Kubernetes IDE — desktop shell or self-hosted web server",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if runDesktop == nil {
				return fmt.Errorf("this build is headless; use `klados serve`")
			}
			return runDesktop(assets)
		},
	}

	root.AddCommand(pluginCmd)
	root.AddCommand(newServeCmd(assets))

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
