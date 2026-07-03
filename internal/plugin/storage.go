package plugin

import (
	"encoding/json"
	"github.com/sasha-s/go-deadlock"
	"os"
	"path/filepath"
	"time"

	"github.com/adrg/xdg"
)

// PluginStorage is a thread-safe, debounced key-value store backed by a JSON file.
// Each plugin gets its own isolated storage at $XDG_DATA_HOME/klados/plugins/{name}/storage.json.
type PluginStorage struct {
	mu       deadlock.RWMutex
	data     map[string]string
	path     string
	dmu      deadlock.Mutex
	debounce *time.Timer
}

func NewPluginStorage(pluginName string) (*PluginStorage, error) {
	dir := filepath.Join(xdg.DataHome, "klados", "plugins", pluginName)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}

	p := filepath.Join(dir, "storage.json")
	s := &PluginStorage{
		data: make(map[string]string),
		path: p,
	}

	data, err := os.ReadFile(p)
	if err == nil {
		_ = json.Unmarshal(data, &s.data)
	}

	return s, nil
}

func (s *PluginStorage) Get(key string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.data[key]
	return v, ok
}

func (s *PluginStorage) Set(key, value string) {
	s.mu.Lock()
	s.data[key] = value
	s.mu.Unlock()
	s.saveDebounced()
}

func (s *PluginStorage) Delete(key string) {
	s.mu.Lock()
	delete(s.data, key)
	s.mu.Unlock()
	s.saveDebounced()
}

func (s *PluginStorage) List() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	keys := make([]string, 0, len(s.data))
	for k := range s.data {
		keys = append(keys, k)
	}
	return keys
}

func (s *PluginStorage) Flush() error {
	s.mu.RLock()
	data, err := json.MarshalIndent(s.data, "", "  ")
	s.mu.RUnlock()
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0o644)
}

func (s *PluginStorage) saveDebounced() {
	s.dmu.Lock()
	defer s.dmu.Unlock()
	if s.debounce != nil {
		s.debounce.Stop()
	}
	s.debounce = time.AfterFunc(500*time.Millisecond, func() {
		_ = s.Flush()
	})
}
