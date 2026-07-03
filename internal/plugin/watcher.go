package plugin

import (
	"context"
	"github.com/sasha-s/go-deadlock"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Vilsol/slox"
	"github.com/fsnotify/fsnotify"
)

// PluginWatcher watches plugin directories for changes and triggers reloads
// via a callback after a 200ms debounce.
type PluginWatcher struct {
	watcher   *fsnotify.Watcher
	onReload  func(name string)
	dirs      map[string]string // pluginDir → pluginName
	debounces map[string]*time.Timer
	mu        deadlock.Mutex
	ctx       context.Context
	done      chan struct{}
}

func NewPluginWatcher(ctx context.Context, onReload func(name string)) (*PluginWatcher, error) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	return &PluginWatcher{
		watcher:   w,
		onReload:  onReload,
		dirs:      make(map[string]string),
		debounces: make(map[string]*time.Timer),
		ctx:       ctx,
		done:      make(chan struct{}),
	}, nil
}

// Watch adds a plugin directory and all its subdirectories to the watch list.
func (w *PluginWatcher) Watch(pluginName, pluginDir string) error {
	slox.Debug(w.ctx, "plugin watcher watching", "plugin", pluginName, "dir", pluginDir)
	w.mu.Lock()
	w.dirs[pluginDir] = pluginName
	w.mu.Unlock()
	return w.watchRecursive(pluginDir)
}

func (w *PluginWatcher) watchRecursive(dir string) error {
	return filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			return w.watcher.Add(path)
		}
		return nil
	})
}

// Unwatch removes a plugin's directory from the watch list.
func (w *PluginWatcher) Unwatch(pluginDir string) {
	w.mu.Lock()
	delete(w.dirs, pluginDir)
	w.mu.Unlock()

	_ = filepath.WalkDir(pluginDir, func(path string, d os.DirEntry, err error) error {
		if err == nil && d.IsDir() {
			_ = w.watcher.Remove(path)
		}
		return nil
	})
}

// Start begins watching for events in a background goroutine.
func (w *PluginWatcher) Start() {
	go w.loop()
}

// Stop shuts down the watcher.
func (w *PluginWatcher) Stop() {
	close(w.done)
	_ = w.watcher.Close()
}

func (w *PluginWatcher) loop() {
	for {
		select {
		case <-w.done:
			return
		case event, ok := <-w.watcher.Events:
			if !ok {
				return
			}
			w.handleEvent(event)
		case err, ok := <-w.watcher.Errors:
			if !ok {
				return
			}
			slox.Warn(w.ctx, "plugin watcher error", "error", err)
		}
	}
}

func (w *PluginWatcher) handleEvent(event fsnotify.Event) {
	// When a new directory is created inside a watched plugin dir, watch it too.
	if event.Op.Has(fsnotify.Create) {
		if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
			_ = w.watcher.Add(event.Name)
		}
	}

	// Ignore runtime-written files that must not trigger a reload.
	base := filepath.Base(event.Name)
	if base == "storage.json" {
		return
	}

	name, ok := w.pluginForPath(event.Name)
	if !ok {
		return
	}

	slox.Debug(w.ctx, "plugin file changed, scheduling reload", "plugin", name, "file", event.Name, "op", event.Op)

	w.mu.Lock()
	if t, exists := w.debounces[name]; exists {
		t.Stop()
	}
	w.debounces[name] = time.AfterFunc(200*time.Millisecond, func() {
		w.onReload(name)
	})
	w.mu.Unlock()
}

func (w *PluginWatcher) pluginForPath(path string) (string, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	for dir, name := range w.dirs {
		if path == dir || strings.HasPrefix(path, dir+string(filepath.Separator)) {
			return name, true
		}
	}
	return "", false
}
