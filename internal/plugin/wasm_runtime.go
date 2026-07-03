package plugin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/sasha-s/go-deadlock"
	"os"
	"path/filepath"

	"github.com/Vilsol/slox"
	"github.com/adrg/xdg"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
	"github.com/tetratelabs/wazero/sys"
)

// WasmRuntime manages a single plugin's Wasm module lifecycle.
type WasmRuntime struct {
	rt         wazero.Runtime
	mod        api.Module
	pluginName string
	ctx        context.Context
	eventCh    chan eventPayload
	hapi       *hostAPI
	mu         deadlock.Mutex // serializes all mod calls
}

// NewWasmRuntime loads and instantiates a Wasm plugin module.
// storage may be nil if the plugin does not declare storage permission.
func NewWasmRuntime(ctx context.Context, wasmBytes []byte, pluginName string, perms PermissionSet, storage *PluginStorage, deps HostAPIDeps) (*WasmRuntime, error) {
	rt := wazero.NewRuntimeWithConfig(ctx, wazero.NewRuntimeConfig())

	modRef := &moduleRef{}
	eventCh := make(chan eventPayload, 64)
	hapi := newHostAPI(ctx, pluginName, perms, modRef, storage, deps, eventCh)

	if err := hapi.Register(rt); err != nil {
		_ = rt.Close(ctx)
		return nil, fmt.Errorf("registering host API: %w", err)
	}

	// Build WASI with a soft proc_exit: exit(0) becomes a no-op so standard Go
	// WASM command modules (which call proc_exit after main()) stay open for
	// further exported-function calls. Non-zero exits still close the module.
	wasiBuilder := rt.NewHostModuleBuilder(wasi_snapshot_preview1.ModuleName)
	wasi_snapshot_preview1.NewFunctionExporter().ExportFunctions(wasiBuilder)
	wasiBuilder.NewFunctionBuilder().
		WithFunc(func(ctx context.Context, mod api.Module, exitCode uint32) {
			if exitCode != 0 {
				_ = mod.CloseWithExitCode(ctx, exitCode)
				panic(sys.NewExitError(exitCode))
			}
			// exit(0): no-op — runtime finished initializing; module stays open.
		}).Export("proc_exit")
	if _, err := wasiBuilder.Instantiate(ctx); err != nil {
		_ = rt.Close(ctx)
		return nil, fmt.Errorf("instantiating WASI: %w", err)
	}

	compiled, err := rt.CompileModule(ctx, wasmBytes)
	if err != nil {
		_ = rt.Close(ctx)
		return nil, fmt.Errorf("compiling wasm: %w", err)
	}

	logWriter := &pluginLogWriter{ctx: ctx, pluginName: pluginName}
	modCfg := wazero.NewModuleConfig().
		WithName(pluginName).
		WithStdout(logWriter).
		WithStderr(logWriter).
		WithStartFunctions() // skip _start; plugin_init serves as the entry point

	if perms.AllowsWasi("env") {
		modCfg = modCfg.WithEnv("KLADOS_PLUGIN", pluginName)
	}

	if perms.AllowsWasi("filesystem") {
		pluginDataDir := filepath.Join(xdg.DataHome, "klados", "plugins", pluginName)
		if err := os.MkdirAll(pluginDataDir, 0o700); err == nil {
			modCfg = modCfg.WithFSConfig(wazero.NewFSConfig().WithDirMount(pluginDataDir, "/data"))
		}
	}

	mod, err := rt.InstantiateModule(ctx, compiled, modCfg)
	if err != nil {
		_ = rt.Close(ctx)
		return nil, fmt.Errorf("instantiating wasm module: %w", err)
	}

	modRef.mod = mod

	// Initialize the guest runtime before calling any exports.
	// Prefer _initialize (WASI reactor) if present; fall back to _start (command).
	// With our no-op proc_exit, _start returns normally after running init().
	if initializeFn := mod.ExportedFunction("_initialize"); initializeFn != nil {
		if _, err := initializeFn.Call(ctx); err != nil {
			_ = mod.Close(ctx)
			_ = rt.Close(ctx)
			return nil, fmt.Errorf("_initialize trap: %w", err)
		}
	} else if startFn := mod.ExportedFunction("_start"); startFn != nil {
		if _, err := startFn.Call(ctx); err != nil {
			var exitErr *sys.ExitError
			if errors.As(err, &exitErr) {
				_ = rt.Close(ctx)
				return nil, fmt.Errorf("_start exited with code %d", exitErr.ExitCode())
			}
			_ = mod.Close(ctx)
			_ = rt.Close(ctx)
			return nil, fmt.Errorf("_start trap: %w", err)
		}
	}

	if initFn := mod.ExportedFunction("plugin_init"); initFn != nil {
		results, err := initFn.Call(ctx)
		if err != nil {
			_ = mod.Close(ctx)
			_ = rt.Close(ctx)
			return nil, fmt.Errorf("plugin_init trap: %w", err)
		}
		if len(results) > 0 && results[0] != 0 {
			_ = mod.Close(ctx)
			_ = rt.Close(ctx)
			return nil, fmt.Errorf("plugin_init returned error code %d", results[0])
		}
	}

	r := &WasmRuntime{
		rt:         rt,
		mod:        mod,
		pluginName: pluginName,
		ctx:        ctx,
		eventCh:    eventCh,
		hapi:       hapi,
	}

	go func() {
		for ev := range eventCh {
			if err := r.CallOnEvent(ev.Name, ev.Data); err != nil {
				slox.Warn(ctx, "plugin event delivery failed", "plugin", pluginName, "event", ev.Name, "error", err)
			}
		}
	}()

	return r, nil
}

// CallEnrich serializes the GVR and object JSON into guest memory, calls
// plugin_enrich, and returns the enriched JSON bytes.
func (r *WasmRuntime) CallEnrich(gvr string, objJSON []byte) ([]byte, error) {
	gvrBytes := []byte(gvr)
	gvrLen := uint64(len(gvrBytes))
	objLen := uint64(len(objJSON))
	totalLen := gvrLen + objLen

	allocFn := r.mod.ExportedFunction("plugin_alloc")
	freeFn := r.mod.ExportedFunction("plugin_free")
	enrichFn := r.mod.ExportedFunction("plugin_enrich")

	if allocFn == nil || enrichFn == nil {
		return nil, fmt.Errorf("plugin %s missing required exports (plugin_alloc, plugin_enrich)", r.pluginName)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Allocate input buffer in guest memory.
	allocResults, err := allocFn.Call(r.ctx, totalLen)
	if err != nil || len(allocResults) == 0 {
		return nil, fmt.Errorf("plugin_alloc failed: %w", err)
	}
	inputPtr := uint32(allocResults[0])

	// Write GVR then object JSON into the allocated buffer.
	if !r.mod.Memory().Write(inputPtr, gvrBytes) {
		return nil, fmt.Errorf("writing gvr to guest memory failed")
	}
	if !r.mod.Memory().Write(inputPtr+uint32(gvrLen), objJSON) {
		return nil, fmt.Errorf("writing obj to guest memory failed")
	}

	// Call plugin_enrich(gvr_ptr, gvr_len, obj_ptr, obj_len) → uint64
	// Return value packs (ptr << 32 | len); zero means empty result.
	enrichResults, err := enrichFn.Call(r.ctx,
		uint64(inputPtr), gvrLen,
		uint64(inputPtr)+gvrLen, objLen,
	)

	// Release input buffer regardless of enrichment success/failure.
	if freeFn != nil {
		_, _ = freeFn.Call(r.ctx, uint64(inputPtr), totalLen)
	}

	if err != nil {
		return nil, fmt.Errorf("plugin_enrich trap: %w", err)
	}
	if len(enrichResults) == 0 {
		return nil, fmt.Errorf("plugin_enrich returned no results")
	}

	packed := enrichResults[0]
	resultPtr := uint32(packed >> 32)
	resultLen := uint32(packed)

	if resultLen == 0 {
		return nil, nil
	}

	// Read result from guest memory.
	result, ok := r.mod.Memory().Read(resultPtr, resultLen)
	if !ok {
		return nil, fmt.Errorf("reading result from guest memory failed")
	}

	// Copy before freeing.
	out := make([]byte, len(result))
	copy(out, result)

	if freeFn != nil {
		_, _ = freeFn.Call(r.ctx, uint64(resultPtr), uint64(resultLen))
	}

	return out, nil
}

// CallOnEvent calls the optional plugin_on_event export with a JSON payload.
// If the export doesn't exist, returns nil (not all plugins handle events).
func (r *WasmRuntime) CallOnEvent(eventName string, payload []byte) error {
	fn := r.mod.ExportedFunction("plugin_on_event")
	if fn == nil {
		return nil
	}

	data, _ := json.Marshal(map[string]any{
		"event":   eventName,
		"payload": json.RawMessage(payload),
	})

	allocFn := r.mod.ExportedFunction("plugin_alloc")
	freeFn := r.mod.ExportedFunction("plugin_free")
	if allocFn == nil {
		return nil
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	results, err := allocFn.Call(r.ctx, uint64(len(data)))
	if err != nil || len(results) == 0 {
		return fmt.Errorf("plugin_alloc failed: %w", err)
	}
	ptr := uint32(results[0])

	if !r.mod.Memory().Write(ptr, data) {
		return fmt.Errorf("writing event to guest memory failed")
	}

	_, err = fn.Call(r.ctx, uint64(ptr), uint64(len(data)))

	if freeFn != nil {
		_, _ = freeFn.Call(r.ctx, uint64(ptr), uint64(len(data)))
	}

	return err
}

// CallCommand calls plugin_command(id_ptr, id_len).
// Returns nil if the export does not exist (component-path plugins won't export it).
func (r *WasmRuntime) CallCommand(commandID string) error {
	fn := r.mod.ExportedFunction("plugin_command")
	if fn == nil {
		slox.Info(r.ctx, "[wasm-cmd] plugin_command export absent", "plugin", r.pluginName, "command", commandID)
		return nil
	}
	slox.Info(r.ctx, "[wasm-cmd] plugin_command export found, calling", "plugin", r.pluginName, "command", commandID)

	idBytes := []byte(commandID)
	allocFn := r.mod.ExportedFunction("plugin_alloc")
	freeFn := r.mod.ExportedFunction("plugin_free")
	if allocFn == nil {
		return fmt.Errorf("plugin %s missing plugin_alloc", r.pluginName)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	results, err := allocFn.Call(r.ctx, uint64(len(idBytes)))
	if err != nil || len(results) == 0 {
		return fmt.Errorf("plugin_alloc failed: %w", err)
	}
	ptr := uint32(results[0])

	if !r.mod.Memory().Write(ptr, idBytes) {
		return fmt.Errorf("writing command id to guest memory failed")
	}

	_, err = fn.Call(r.ctx, uint64(ptr), uint64(len(idBytes)))

	if freeFn != nil {
		_, _ = freeFn.Call(r.ctx, uint64(ptr), uint64(len(idBytes)))
	}
	return err
}

// Close calls plugin_destroy and shuts down the runtime.
func (r *WasmRuntime) Close() error {
	if destroyFn := r.mod.ExportedFunction("plugin_destroy"); destroyFn != nil {
		_, _ = destroyFn.Call(r.ctx)
	}
	if r.hapi != nil {
		r.hapi.Close()
	}
	if r.eventCh != nil {
		close(r.eventCh)
	}
	return r.rt.Close(r.ctx)
}

// pluginLogWriter routes Wasm stdout/stderr to slox line by line.
type pluginLogWriter struct {
	ctx        context.Context
	pluginName string
	buf        bytes.Buffer
}

func (w *pluginLogWriter) Write(p []byte) (int, error) {
	w.buf.Write(p)
	data := w.buf.Bytes()
	for {
		idx := bytes.IndexByte(data, '\n')
		if idx == -1 {
			break
		}
		line := string(data[:idx])
		if line != "" {
			slox.Info(w.ctx, line, "plugin", w.pluginName)
		}
		data = data[idx+1:]
	}
	w.buf.Reset()
	w.buf.Write(data)
	return len(p), nil
}
