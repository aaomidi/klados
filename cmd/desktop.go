//go:build !headless

package cmd

import (
	"embed"
	"fmt"
	"io/fs"
	"log"
	"os"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"github.com/Vilsol/klados/internal/config"
	"github.com/Vilsol/klados/internal/logging"
	"github.com/Vilsol/klados/internal/server"
	"github.com/Vilsol/klados/internal/session"
)

func init() {
	runDesktop = desktopMain
}

// wailsDesktop implements server.Desktop with real native integrations:
// file dialogs and pop-out OS windows for bottom-panel tabs.
type wailsDesktop struct {
	app       *application.App
	hub       *server.Hub
	serverURL string
	mu        sync.Mutex
}

func (d *wailsDesktop) BrowseKubeconfigFile() (string, error) {
	return d.app.Dialog.OpenFile().
		AddFilter("Kubeconfig files", "*.yaml").
		PromptForSingleSelection()
}

func (d *wailsDesktop) BrowsePluginFile() (string, error) {
	return d.app.Dialog.OpenFile().
		AddFilter("Klados plugin archives", "*.oci.tar.gz").
		PromptForSingleSelection()
}

func (d *wailsDesktop) BrowseManifestFile() (string, error) {
	path, err := d.app.Dialog.OpenFile().
		AddFilter("YAML files", "*.yaml;*.yml").
		PromptForSingleSelection()
	if err != nil || path == "" {
		return "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (d *wailsDesktop) OpenURL(url string) error {
	return d.app.Browser.OpenURL(url)
}

func (d *wailsDesktop) OpenPanelWindow(panelID, title string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	windowName := fmt.Sprintf("panel-%s", panelID)

	if existing, ok := d.app.Window.GetByName(windowName); ok {
		existing.Focus()
		return nil
	}

	win := d.app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:   windowName,
		Title:  title,
		Width:  800,
		Height: 500,
		URL:    fmt.Sprintf("%s/?panel=%s", d.serverURL, panelID),
	})

	win.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		d.hub.Emit("panel:closed", panelID)
	})

	win.Show()
	return nil
}

// desktopMain runs the unified server on loopback and wraps it in a Wails
// window. The webview loads the same SPA over the same transport the hosted
// deployment uses — Wails provides the native window, dialogs, and pop-outs.
func desktopMain(assets embed.FS) error {
	ctx := logging.Setup()

	cfg, err := config.Load()
	if err != nil {
		log.Fatal("failed to load config:", err)
	}
	sess, err := session.Load()
	if err != nil {
		log.Fatal("failed to load session:", err)
	}

	hub := server.NewHub(ctx)
	deps, err := server.Bootstrap(ctx, hub, cfg, sess)
	if err != nil {
		return err
	}
	defer deps.Shutdown()

	staticAssets, err := fs.Sub(assets, "frontend/dist")
	if err != nil {
		return fmt.Errorf("embedded frontend assets missing: %w", err)
	}

	app := application.New(application.Options{
		Name:        "klados",
		Description: "Kubernetes Desktop IDE",
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})

	desktop := &wailsDesktop{app: app, hub: hub}

	srv := server.New(ctx, hub, deps, server.Options{
		Addr:    "127.0.0.1:0",
		Assets:  staticAssets,
		Desktop: desktop,
		// Set by `wails3 dev`; routes the frontend through Vite for HMR
		// while the API stays same-origin on the local server.
		DevServerURL: os.Getenv("FRONTEND_DEVSERVER_URL"),
		Capabilities: server.Capabilities{
			PortForwarding: true,
			OSWindows:      true,
			NativeDialogs:  true,
			Mode:           "desktop",
		},
	})
	if err := srv.Start(ctx); err != nil {
		return err
	}
	defer func() { _ = srv.Stop() }()
	desktop.serverURL = fmt.Sprintf("http://127.0.0.1:%d", srv.Port())

	winOpts := application.WebviewWindowOptions{
		Title:           "Klados",
		DevToolsEnabled: true,
		Mac: application.MacWindow{
			InvisibleTitleBarHeight: 50,
			Backdrop:                application.MacBackdropTranslucent,
			TitleBar:                application.MacTitleBarHiddenInset,
		},
		BackgroundColour: application.NewRGB(27, 38, 54),
		URL:              desktop.serverURL + "/",
	}

	if sess.Window.Width > 0 {
		winOpts.Width = sess.Window.Width
	}
	if sess.Window.Height > 0 {
		winOpts.Height = sess.Window.Height
	}
	if sess.Window.X > 0 || sess.Window.Y > 0 {
		winOpts.X = sess.Window.X
		winOpts.Y = sess.Window.Y
	}

	win := app.Window.NewWithOptions(winOpts)

	// Persist main-window geometry so the next launch restores it.
	win.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		w, h := win.Size()
		x, y := win.Position()
		sess.Window = session.WindowState{X: x, Y: y, Width: w, Height: h}
		_ = sess.Save()
	})

	return app.Run()
}
