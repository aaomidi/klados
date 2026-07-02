package plugin_test

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/MarvinJWendt/testza"
	"github.com/adrg/xdg"

	"github.com/Vilsol/klados/internal/plugin"
)

func newTestStorage(t *testing.T) *plugin.PluginStorage {
	t.Helper()
	// Override xdg.DataHome by writing directly — use the exported constructor
	// with a temp-dir backed plugin name that won't collide.
	name := "test-storage-" + t.Name()
	// Patch: storage path is under xdg.DataHome which we can't easily override,
	// but we can use a unique name per test so they don't interfere.
	st, err := plugin.NewPluginStorage(name)
	testza.AssertNil(t, err)
	t.Cleanup(func() {
		p := storagePathForName(name)
		_ = os.RemoveAll(filepath.Dir(p))
	})
	return st
}

// storagePathForName mirrors the internal path construction for test cleanup.
func storagePathForName(name string) string {
	return filepath.Join(xdg.DataHome, "klados", "plugins", name, "storage.json")
}

func TestPluginStorage_GetSetDelete(t *testing.T) {
	st := newTestStorage(t)

	// Missing key returns false
	_, found := st.Get("k1")
	testza.AssertFalse(t, found)

	// Set then Get
	st.Set("k1", "v1")
	val, found := st.Get("k1")
	testza.AssertTrue(t, found)
	testza.AssertEqual(t, "v1", val)

	// Overwrite
	st.Set("k1", "v2")
	val, found = st.Get("k1")
	testza.AssertTrue(t, found)
	testza.AssertEqual(t, "v2", val)

	// Delete
	st.Delete("k1")
	_, found = st.Get("k1")
	testza.AssertFalse(t, found)
}

func TestPluginStorage_List(t *testing.T) {
	st := newTestStorage(t)

	testza.AssertEqual(t, 0, len(st.List()))

	st.Set("a", "1")
	st.Set("b", "2")
	st.Set("c", "3")

	keys := st.List()
	testza.AssertEqual(t, 3, len(keys))
}

func TestPluginStorage_ConcurrentAccess(t *testing.T) {
	st := newTestStorage(t)

	var wg sync.WaitGroup
	for i := range 20 {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			key := "key"
			st.Set(key, "value")
			_, _ = st.Get(key)
			_ = st.List()
			if n%5 == 0 {
				st.Delete(key)
			}
		}(i)
	}
	wg.Wait()
}

func TestPluginStorage_Flush(t *testing.T) {
	name := "test-flush-" + t.Name()
	st, err := plugin.NewPluginStorage(name)
	testza.AssertNil(t, err)
	t.Cleanup(func() {
		p := storagePathForName(name)
		_ = os.RemoveAll(filepath.Dir(p))
	})

	st.Set("persist", "yes")
	testza.AssertNil(t, st.Flush())

	// Re-load from disk
	st2, err := plugin.NewPluginStorage(name)
	testza.AssertNil(t, err)
	val, found := st2.Get("persist")
	testza.AssertTrue(t, found)
	testza.AssertEqual(t, "yes", val)
}

func TestPluginStorage_SeparatePerPlugin(t *testing.T) {
	st1 := newTestStorage(t)
	st2, err := plugin.NewPluginStorage("test-storage-other-" + t.Name())
	testza.AssertNil(t, err)
	t.Cleanup(func() {
		p := storagePathForName("test-storage-other-" + t.Name())
		_ = os.RemoveAll(filepath.Dir(p))
	})

	st1.Set("shared_key", "from_plugin_a")
	_, found := st2.Get("shared_key")
	testza.AssertFalse(t, found)
}
