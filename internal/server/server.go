package server

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path"
	"strings"
	"time"

	"github.com/Vilsol/slox"
	"github.com/gorilla/websocket"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"

	"github.com/Vilsol/klados/gen/klados/v1/kladosv1connect"
	"github.com/Vilsol/klados/internal/services"
)

// Options configures one server instance. The same server backs both
// deployment shapes: the hosted split (public addr, embedded SPA) and the
// desktop shell (loopback addr, webview pointed at the local URL).
type Options struct {
	// Addr to bind, e.g. ":8080". Empty binds 127.0.0.1:0 (desktop mode).
	Addr string
	// Assets is the built SPA (frontend/dist). Nil disables static serving
	// (dev mode, where Vite serves the frontend).
	Assets fs.FS
	// AllowedOrigins for CORS/WS upgrades. Same-origin requests always pass;
	// list the Vite dev origin here during development. "*" allows all.
	AllowedOrigins []string
	Capabilities   Capabilities
	// TLSCertFile/TLSKeyFile enable HTTPS. Browsers only negotiate HTTP/2
	// over TLS (ALPN), so this is what unlocks h2 for browser clients;
	// without it they fall back to HTTP/1.1 (which the app handles fine —
	// the frontend multiplexes all event subscriptions over one stream).
	TLSCertFile string
	TLSKeyFile  string
	// Desktop provides native integrations (dialogs, OS windows). Nil in
	// server mode.
	Desktop Desktop
	// DevServerURL, when set (wails3 dev exports FRONTEND_DEVSERVER_URL),
	// reverse-proxies non-API requests to the Vite dev server instead of
	// serving Assets — hot reload with the API staying same-origin.
	DevServerURL string
}

// Deps carries the service singletons the handlers adapt.
type Deps struct {
	App           *services.AppService
	Cluster       *services.ClusterService
	Config        *services.ConfigService
	Resource      *services.ResourceService
	Schema        *services.SchemaService
	Log           *services.LogService
	Exec          *services.ExecService
	PortForward   *services.PortForwardService
	VolumeBrowser *services.VolumeBrowserService
	Drain         *services.DrainService
	Metrics       *services.MetricsService
	Plugin        *services.PluginService
	Helm          *services.HelmService
}

type Server struct {
	hub  *Hub
	deps Deps
	opts Options
	ctx  context.Context

	httpSrv *http.Server
	port    int
}

func New(ctx context.Context, hub *Hub, deps Deps, opts Options) *Server {
	return &Server{hub: hub, deps: deps, opts: opts, ctx: ctx}
}

func (s *Server) Port() int { return s.port }

func (s *Server) Start(ctx context.Context) error {
	addr := s.opts.Addr
	if addr == "" {
		addr = "127.0.0.1:0"
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listening on %s: %w", addr, err)
	}
	s.port = ln.Addr().(*net.TCPAddr).Port

	mux := http.NewServeMux()
	s.registerConnect(mux)
	s.registerStreams(mux)
	s.registerStatic(mux)

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})
	// Frontend console forwarding (main.ts monkey-patches console.*).
	mux.HandleFunc("POST /log", func(w http.ResponseWriter, r *http.Request) {
		body := make([]byte, 0, 512)
		buf := make([]byte, 512)
		for len(body) < 64*1024 {
			n, readErr := r.Body.Read(buf)
			body = append(body, buf[:n]...)
			if readErr != nil {
				break
			}
		}
		slox.Info(s.ctx, "frontend", "msg", string(body))
		w.WriteHeader(http.StatusNoContent)
	})

	handler := s.cors(mux)
	useTLS := s.opts.TLSCertFile != "" && s.opts.TLSKeyFile != ""
	if useTLS {
		// net/http enables HTTP/2 automatically on TLS listeners (ALPN).
		s.httpSrv = &http.Server{
			Handler:           handler,
			ReadHeaderTimeout: 10 * time.Second,
		}
	} else {
		// h2c lets proxies that terminate TLS upstream (or curl-style
		// clients) speak HTTP/2 without TLS at this hop. Browsers never do
		// h2c and use HTTP/1.1 here.
		h2s := &http2.Server{IdleTimeout: 5 * time.Minute}
		s.httpSrv = &http.Server{
			Handler:           h2c.NewHandler(handler, h2s),
			ReadHeaderTimeout: 10 * time.Second,
		}
	}

	slox.Info(s.ctx, "server started", "addr", ln.Addr().String(), "tls", useTLS)

	go func() {
		var err error
		if useTLS {
			err = s.httpSrv.ServeTLS(ln, s.opts.TLSCertFile, s.opts.TLSKeyFile)
		} else {
			err = s.httpSrv.Serve(ln)
		}
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			slox.Error(s.ctx, "server error", "error", err)
		}
	}()
	go func() {
		<-ctx.Done()
		_ = s.Stop()
	}()
	return nil
}

func (s *Server) Stop() error {
	if s.httpSrv == nil {
		return nil
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return s.httpSrv.Shutdown(shutdownCtx)
}

func (s *Server) registerConnect(mux *http.ServeMux) {
	register := func(pattern string, handler http.Handler) {
		mux.Handle(pattern, handler)
	}
	register(kladosv1connect.NewEventServiceHandler(NewEventHandler(s.hub)))
	register(kladosv1connect.NewAppServiceHandler(NewAppHandler(s.deps.App, s.opts.Capabilities, s.opts.Desktop)))
	register(kladosv1connect.NewWindowServiceHandler(NewWindowHandler(s.opts.Desktop)))
	register(kladosv1connect.NewClusterServiceHandler(NewClusterHandler(s.deps.Cluster)))
	register(kladosv1connect.NewConfigServiceHandler(NewConfigHandler(s.deps.Config)))
	register(kladosv1connect.NewResourceServiceHandler(NewResourceHandler(s.deps.Resource)))
	register(kladosv1connect.NewSchemaServiceHandler(NewSchemaHandler(s.deps.Schema)))
	register(kladosv1connect.NewLogServiceHandler(NewLogHandler(s.deps.Log)))
	register(kladosv1connect.NewExecServiceHandler(NewExecHandler(s.deps.Exec)))
	register(kladosv1connect.NewPortForwardServiceHandler(NewPortForwardHandler(s.deps.PortForward)))
	register(kladosv1connect.NewVolumeBrowserServiceHandler(NewVolumeBrowserHandler(s.deps.VolumeBrowser)))
	register(kladosv1connect.NewDrainServiceHandler(NewDrainHandler(s.deps.Drain)))
	register(kladosv1connect.NewMetricsServiceHandler(NewMetricsHandler(s.deps.Metrics)))
	register(kladosv1connect.NewPluginServiceHandler(NewPluginHandler(s.deps.Plugin)))
	register(kladosv1connect.NewHelmServiceHandler(NewHelmHandler(s.deps.Helm)))
}

func (s *Server) registerStreams(mux *http.ServeMux) {
	upgrader := websocket.Upgrader{
		ReadBufferSize:    4096,
		WriteBufferSize:   4096,
		EnableCompression: true,
		CheckOrigin:       func(r *http.Request) bool { return s.originAllowed(r) },
	}

	mux.HandleFunc("GET /ws/logs/{streamID}", func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		if streamer := s.deps.App.LogStreamer(); streamer != nil {
			streamer.HandleConn(r.PathValue("streamID"), conn)
		}
	})

	mux.HandleFunc("GET /ws/exec/{sessionID}", func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		if mgr := s.deps.App.ExecManager(); mgr != nil {
			mgr.HandleConn(r.PathValue("sessionID"), conn)
		}
	})

	// Plugin JS modules, dynamically import()ed by the frontend.
	mux.HandleFunc("GET /plugins/", func(w http.ResponseWriter, r *http.Request) {
		dir := s.deps.App.PluginsDir()
		if dir == "" {
			http.NotFound(w, r)
			return
		}
		rel := strings.TrimPrefix(r.URL.Path, "/plugins/")
		switch path.Ext(rel) {
		case ".js", ".mjs":
			w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
		case ".map":
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
		}
		w.Header().Set("Access-Control-Allow-Origin", "*")
		http.ServeFileFS(w, r, os.DirFS(dir), rel)
	})
}

func (s *Server) registerStatic(mux *http.ServeMux) {
	if s.opts.DevServerURL != "" {
		target, err := url.Parse(s.opts.DevServerURL)
		if err != nil {
			slox.Error(s.ctx, "invalid dev server url", "url", s.opts.DevServerURL, "error", err)
			return
		}
		// Everything that isn't an API route goes to Vite — including its
		// HMR websocket, which httputil.ReverseProxy upgrades transparently.
		proxy := httputil.NewSingleHostReverseProxy(target)
		mux.Handle("/", proxy)
		slox.Info(s.ctx, "proxying frontend to dev server", "url", s.opts.DevServerURL)
		return
	}
	if s.opts.Assets == nil {
		return
	}
	fileServer := http.FileServerFS(s.opts.Assets)
	// Registered without a method so it stays strictly less specific than the
	// Connect service patterns ("/klados.v1.X/"), which ServeMux requires.
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		p := strings.TrimPrefix(r.URL.Path, "/")
		if p != "" {
			if _, err := fs.Stat(s.opts.Assets, p); err == nil {
				fileServer.ServeHTTP(w, r)
				return
			}
		}
		// SPA fallback: any unknown extension-less path serves index.html so
		// client-side routes deep-link correctly.
		if path.Ext(p) != "" {
			http.NotFound(w, r)
			return
		}
		r2 := r.Clone(r.Context())
		r2.URL.Path = "/"
		fileServer.ServeHTTP(w, r2)
	})
}

func (s *Server) originAllowed(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true // non-browser or same-origin fetch without Origin
	}
	host := r.Host
	if strings.EqualFold(strings.TrimPrefix(strings.TrimPrefix(origin, "http://"), "https://"), host) {
		return true
	}
	for _, allowed := range s.opts.AllowedOrigins {
		if allowed == "*" || strings.EqualFold(allowed, origin) {
			return true
		}
	}
	return false
}

// cors handles preflights and reflects allowed origins so a cross-origin dev
// frontend (Vite) can call Connect endpoints and POST /log.
func (s *Server) cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" && s.originAllowed(r) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			if r.Method == http.MethodOptions {
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Connect-Protocol-Version, Connect-Timeout-Ms, Accept-Encoding")
				w.Header().Set("Access-Control-Expose-Headers", "Connect-Protocol-Version")
				w.Header().Set("Access-Control-Max-Age", "86400")
				w.WriteHeader(http.StatusNoContent)
				return
			}
		} else if origin != "" && !s.originAllowed(r) {
			http.Error(w, "origin not allowed", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}
