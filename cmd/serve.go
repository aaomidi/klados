package cmd

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"os/signal"
	"syscall"

	"github.com/Vilsol/slox"
	"github.com/spf13/cobra"

	"github.com/Vilsol/klados/internal/config"
	"github.com/Vilsol/klados/internal/logging"
	"github.com/Vilsol/klados/internal/server"
	"github.com/Vilsol/klados/internal/session"
)

func newServeCmd(assets embed.FS) *cobra.Command {
	var (
		addr           string
		allowedOrigins []string
		noAssets       bool
		tlsCert        string
		tlsKey         string
	)

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Run klados as a headless web server (Kubernetes/remote hosting)",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := logging.Setup()

			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("loading config: %w", err)
			}
			sess, err := session.Load()
			if err != nil {
				return fmt.Errorf("loading session: %w", err)
			}

			hub := server.NewHub(ctx)
			deps, err := server.Bootstrap(ctx, hub, cfg, sess)
			if err != nil {
				return err
			}
			defer deps.Shutdown()

			var staticAssets fs.FS
			if !noAssets {
				sub, subErr := fs.Sub(assets, "frontend/dist")
				if subErr != nil {
					return fmt.Errorf("embedded frontend assets missing: %w", subErr)
				}
				staticAssets = sub
			}

			srv := server.New(ctx, hub, deps, server.Options{
				Addr:           addr,
				Assets:         staticAssets,
				AllowedOrigins: allowedOrigins,
				TLSCertFile:    tlsCert,
				TLSKeyFile:     tlsKey,
				Capabilities: server.Capabilities{
					// Port-forwarding binds the *server's* loopback — useless
					// from a remote browser, so it's advertised off.
					PortForwarding: false,
					OSWindows:      false,
					NativeDialogs:  false,
					Mode:           "server",
				},
			})
			if err := srv.Start(ctx); err != nil {
				return err
			}
			slox.Info(ctx, "klados server ready", "addr", addr)

			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
			<-sigCh
			slox.Info(ctx, "shutting down")
			return srv.Stop()
		},
	}

	cmd.Flags().StringVar(&addr, "addr", ":8080", "address to listen on")
	cmd.Flags().StringSliceVar(&allowedOrigins, "allowed-origin", nil, "additional CORS/WebSocket origins (e.g. the Vite dev server); '*' allows all")
	cmd.Flags().BoolVar(&noAssets, "no-assets", false, "do not serve the embedded frontend (API only; use with an external frontend)")
	cmd.Flags().StringVar(&tlsCert, "tls-cert", "", "TLS certificate file; enables HTTPS and browser HTTP/2 (pair with --tls-key)")
	cmd.Flags().StringVar(&tlsKey, "tls-key", "", "TLS private key file")
	return cmd
}
