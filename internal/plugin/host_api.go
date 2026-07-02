package plugin

import (
	"context"
	"encoding/json"

	"github.com/Vilsol/slox"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"

	"github.com/Vilsol/klados/internal/exec"
	"github.com/Vilsol/klados/internal/logs"
	"github.com/Vilsol/klados/internal/resource"
	"github.com/Vilsol/klados/internal/watcher"
)

// HostAPIDeps holds service dependencies injected into each plugin's host API.
type HostAPIDeps struct {
	ResourceEngine   *resource.ResourceEngine
	WatchManager     *watcher.WatchManager
	LogStreamer      *logs.Streamer
	ExecManager      *exec.Manager
	TemplateRegistry *resource.TemplateRegistry
	GetActiveContext func() string
	// On subscribes to the app event hub; the callback receives the
	// JSON-encoded payload. Returns an unsubscribe func.
	On func(name string, cb func(payloadJSON []byte)) func()
}

type eventPayload struct {
	Name string
	Data []byte
}

// moduleRef is a late-binding holder so host functions can call back into
// the guest module after it is instantiated.
type moduleRef struct {
	mod api.Module
}

type hostAPI struct {
	pluginName  string
	perms       PermissionSet
	modRef      *moduleRef
	storage     *PluginStorage
	ctx         context.Context
	deps        HostAPIDeps
	eventCh     chan<- eventPayload
	cleanupFns  []func()
	pendingResp []byte
}

func newHostAPI(ctx context.Context, pluginName string, perms PermissionSet, modRef *moduleRef, storage *PluginStorage, deps HostAPIDeps, eventCh chan<- eventPayload) *hostAPI {
	return &hostAPI{
		pluginName: pluginName,
		perms:      perms,
		modRef:     modRef,
		storage:    storage,
		ctx:        ctx,
		deps:       deps,
		eventCh:    eventCh,
	}
}

// Close calls all registered cleanup functions.
func (h *hostAPI) Close() {
	for _, fn := range h.cleanupFns {
		fn()
	}
	h.cleanupFns = nil
}

// Register adds the "klados_host" import module to rt.
func (h *hostAPI) Register(rt wazero.Runtime) error {
	b := rt.NewHostModuleBuilder("klados_host")

	// host_log(level i32, msg_ptr i32, msg_len i32)
	b.NewFunctionBuilder().
		WithGoModuleFunction(api.GoModuleFunc(h.hostLog),
			[]api.ValueType{api.ValueTypeI32, api.ValueTypeI32, api.ValueTypeI32},
			[]api.ValueType{}).
		Export("host_log")

	// host_alloc(size i32) i32
	b.NewFunctionBuilder().
		WithGoModuleFunction(api.GoModuleFunc(h.hostAlloc),
			[]api.ValueType{api.ValueTypeI32},
			[]api.ValueType{api.ValueTypeI32}).
		Export("host_alloc")

	// host_free(ptr i32, size i32)
	b.NewFunctionBuilder().
		WithGoModuleFunction(api.GoModuleFunc(h.hostFree),
			[]api.ValueType{api.ValueTypeI32, api.ValueTypeI32},
			[]api.ValueType{}).
		Export("host_free")

	// host_call(method_ptr i32, method_len i32, req_ptr i32, req_len i32) i32
	// Returns response length; 0 = no response. Call host_read_response to retrieve bytes.
	b.NewFunctionBuilder().
		WithGoModuleFunction(api.GoModuleFunc(h.hostCall),
			[]api.ValueType{api.ValueTypeI32, api.ValueTypeI32, api.ValueTypeI32, api.ValueTypeI32},
			[]api.ValueType{api.ValueTypeI32}).
		Export("host_call")

	// host_read_response(buf_ptr i32, buf_len i32)
	// Copies the pending response into the guest-provided buffer.
	b.NewFunctionBuilder().
		WithGoModuleFunction(api.GoModuleFunc(h.hostReadResponse),
			[]api.ValueType{api.ValueTypeI32, api.ValueTypeI32},
			[]api.ValueType{}).
		Export("host_read_response")

	_, err := b.Instantiate(h.ctx)
	return err
}

// hostLog reads a string from guest memory and routes it to slox.
// stack: [level, msg_ptr, msg_len]
func (h *hostAPI) hostLog(_ context.Context, mod api.Module, stack []uint64) {
	level := uint32(stack[0])
	msgPtr := uint32(stack[1])
	msgLen := uint32(stack[2])

	b, ok := mod.Memory().Read(msgPtr, msgLen)
	if !ok {
		return
	}
	msg := string(b)

	switch level {
	case 0:
		slox.Debug(h.ctx, msg, "plugin", h.pluginName)
	case 1:
		slox.Info(h.ctx, msg, "plugin", h.pluginName)
	case 2:
		slox.Warn(h.ctx, msg, "plugin", h.pluginName)
	default:
		slox.Error(h.ctx, msg, "plugin", h.pluginName)
	}
}

// hostAlloc calls plugin_alloc to allocate memory in the guest's address space.
// stack in: [size], stack out: [ptr]
func (h *hostAPI) hostAlloc(ctx context.Context, _ api.Module, stack []uint64) {
	size := stack[0]
	if h.modRef.mod == nil {
		stack[0] = 0
		return
	}
	results, err := h.modRef.mod.ExportedFunction("plugin_alloc").Call(ctx, size)
	if err != nil || len(results) == 0 {
		stack[0] = 0
		return
	}
	stack[0] = results[0]
}

// hostFree calls plugin_free to release memory in the guest's address space.
// stack in: [ptr, size]
func (h *hostAPI) hostFree(ctx context.Context, _ api.Module, stack []uint64) {
	ptr := stack[0]
	size := stack[1]
	if h.modRef.mod == nil {
		return
	}
	_, _ = h.modRef.mod.ExportedFunction("plugin_free").Call(ctx, ptr, size)
}

// hostCall dispatches method calls from the guest to host services.
// stack in: [method_ptr, method_len, req_ptr, req_len]
// stack out: [resp_len] — guest must call host_read_response to retrieve bytes.
func (h *hostAPI) hostCall(_ context.Context, mod api.Module, stack []uint64) {
	methodPtr := uint32(stack[0])
	methodLen := uint32(stack[1])
	reqPtr := uint32(stack[2])
	reqLen := uint32(stack[3])

	methodBytes, _ := mod.Memory().Read(methodPtr, methodLen)
	method := string(methodBytes)

	var reqBytes []byte
	if reqLen > 0 {
		reqBytes, _ = mod.Memory().Read(reqPtr, reqLen)
	}

	h.pendingResp = h.dispatch(method, reqBytes)
	stack[0] = uint64(len(h.pendingResp))
}

// hostReadResponse copies the pending response into the guest-provided buffer.
// stack in: [buf_ptr, buf_len]
func (h *hostAPI) hostReadResponse(_ context.Context, mod api.Module, stack []uint64) {
	bufPtr := uint32(stack[0])
	bufLen := uint32(stack[1])
	if len(h.pendingResp) == 0 || bufLen == 0 {
		return
	}
	n := uint32(len(h.pendingResp))
	if bufLen < n {
		n = bufLen
	}
	mod.Memory().Write(bufPtr, h.pendingResp[:n])
	h.pendingResp = nil
}

func (h *hostAPI) dispatch(method string, reqBytes []byte) []byte {
	var req map[string]any
	_ = json.Unmarshal(reqBytes, &req)

	switch method {
	case "k8s.list":
		gvr, _ := req["gvr"].(string)
		if err := CheckPermission(h.perms, method, gvr, "list"); err != nil {
			slox.Warn(h.ctx, "plugin k8s call denied", "plugin", h.pluginName, "method", method, "gvr", gvr)
			return errorJSON(err.Error())
		}
		if h.deps.ResourceEngine == nil {
			return errorJSON("resource engine not available")
		}
		ns, _ := req["namespace"].(string)
		ctxName := h.deps.GetActiveContext()
		if ctxName == "" {
			return errorJSON("no active cluster context")
		}
		items, err := h.deps.ResourceEngine.ListRaw(h.ctx, ctxName, gvr, ns)
		if err != nil {
			return errorJSON(err.Error())
		}
		return marshalJSON(map[string]any{"items": items})

	case "k8s.get":
		gvr, _ := req["gvr"].(string)
		if err := CheckPermission(h.perms, method, gvr, "get"); err != nil {
			slox.Warn(h.ctx, "plugin k8s call denied", "plugin", h.pluginName, "method", method, "gvr", gvr)
			return errorJSON(err.Error())
		}
		if h.deps.ResourceEngine == nil {
			return errorJSON("resource engine not available")
		}
		ns, _ := req["namespace"].(string)
		name, _ := req["name"].(string)
		ctxName := h.deps.GetActiveContext()
		if ctxName == "" {
			return errorJSON("no active cluster context")
		}
		item, err := h.deps.ResourceEngine.Get(h.ctx, ctxName, gvr, ns, name)
		if err != nil {
			return errorJSON(err.Error())
		}
		return marshalJSON(map[string]any{"object": item})

	case "k8s.create":
		gvr, _ := req["gvr"].(string)
		if err := CheckPermission(h.perms, method, gvr, "create"); err != nil {
			slox.Warn(h.ctx, "plugin k8s call denied", "plugin", h.pluginName, "method", method, "gvr", gvr)
			return errorJSON(err.Error())
		}
		if h.deps.ResourceEngine == nil {
			return errorJSON("resource engine not available")
		}
		ns, _ := req["namespace"].(string)
		obj, _ := req["object"].(map[string]any)
		ctxName := h.deps.GetActiveContext()
		if ctxName == "" {
			return errorJSON("no active cluster context")
		}
		result, err := h.deps.ResourceEngine.Create(h.ctx, ctxName, gvr, ns, obj)
		if err != nil {
			return errorJSON(err.Error())
		}
		return marshalJSON(map[string]any{"object": result})

	case "k8s.update":
		gvr, _ := req["gvr"].(string)
		if err := CheckPermission(h.perms, method, gvr, "update"); err != nil {
			slox.Warn(h.ctx, "plugin k8s call denied", "plugin", h.pluginName, "method", method, "gvr", gvr)
			return errorJSON(err.Error())
		}
		if h.deps.ResourceEngine == nil {
			return errorJSON("resource engine not available")
		}
		ns, _ := req["namespace"].(string)
		obj, _ := req["object"].(map[string]any)
		ctxName := h.deps.GetActiveContext()
		if ctxName == "" {
			return errorJSON("no active cluster context")
		}
		result, err := h.deps.ResourceEngine.Update(h.ctx, ctxName, gvr, ns, obj)
		if err != nil {
			return errorJSON(err.Error())
		}
		return marshalJSON(map[string]any{"object": result})

	case "k8s.delete":
		gvr, _ := req["gvr"].(string)
		if err := CheckPermission(h.perms, method, gvr, "delete"); err != nil {
			slox.Warn(h.ctx, "plugin k8s call denied", "plugin", h.pluginName, "method", method, "gvr", gvr)
			return errorJSON(err.Error())
		}
		if h.deps.ResourceEngine == nil {
			return errorJSON("resource engine not available")
		}
		ns, _ := req["namespace"].(string)
		name, _ := req["name"].(string)
		ctxName := h.deps.GetActiveContext()
		if ctxName == "" {
			return errorJSON("no active cluster context")
		}
		if err := h.deps.ResourceEngine.Delete(h.ctx, ctxName, gvr, ns, name); err != nil {
			return errorJSON(err.Error())
		}
		return marshalJSON(map[string]any{"ok": true})

	case "k8s.watch":
		gvr, _ := req["gvr"].(string)
		if err := CheckPermission(h.perms, method, gvr, "watch"); err != nil {
			slox.Warn(h.ctx, "plugin k8s call denied", "plugin", h.pluginName, "method", method, "gvr", gvr)
			return errorJSON(err.Error())
		}
		if h.deps.WatchManager == nil {
			return errorJSON("watch manager not available")
		}
		ns, _ := req["namespace"].(string)
		ctxName := h.deps.GetActiveContext()
		if ctxName == "" {
			return errorJSON("no active cluster context")
		}
		if err := h.deps.WatchManager.StartWatch(ctxName, gvr, ns, ""); err != nil {
			return errorJSON(err.Error())
		}
		if h.eventCh != nil && h.deps.On != nil {
			watchEvent := "watch:" + ctxName + ":" + gvr + ":" + ns
			unsub := h.deps.On(watchEvent, func(payloadJSON []byte) {
				select {
				case h.eventCh <- eventPayload{Name: watchEvent, Data: payloadJSON}:
				default:
				}
			})
			h.cleanupFns = append(h.cleanupFns, unsub)
			ctxCopy, gvrCopy, nsCopy := ctxName, gvr, ns
			h.cleanupFns = append(h.cleanupFns, func() {
				h.deps.WatchManager.StopWatch(ctxCopy, gvrCopy, nsCopy)
			})
		}
		return marshalJSON(map[string]any{"ok": true})

	case "logs.stream":
		if !h.perms.AllowsLogs() {
			return errorJSON("method not available: " + method)
		}
		if h.deps.LogStreamer == nil {
			return errorJSON("log streamer not available")
		}
		pod, _ := req["pod"].(string)
		ns, _ := req["namespace"].(string)
		container, _ := req["container"].(string)
		ctxName := h.deps.GetActiveContext()
		if ctxName == "" {
			return errorJSON("no active cluster context")
		}
		var tailLines *int64
		if tl, ok := req["tailLines"].(float64); ok {
			n := int64(tl)
			tailLines = &n
		}
		streamID, err := h.deps.LogStreamer.StartStream(ctxName, ns, pod, logs.LogOptions{
			Container: container,
			Follow:    req["follow"] == true,
			Previous:  req["previous"] == true,
			TailLines: tailLines,
		})
		if err != nil {
			return errorJSON(err.Error())
		}
		return marshalJSON(map[string]any{"streamId": streamID})

	case "exec.open":
		if !h.perms.AllowsExec() {
			return errorJSON("method not available: " + method)
		}
		if h.deps.ExecManager == nil {
			return errorJSON("exec manager not available")
		}
		pod, _ := req["pod"].(string)
		ns, _ := req["namespace"].(string)
		container, _ := req["container"].(string)
		ctxName := h.deps.GetActiveContext()
		if ctxName == "" {
			return errorJSON("no active cluster context")
		}
		var shell string
		if cmds, ok := req["command"].([]any); ok && len(cmds) > 0 {
			shell, _ = cmds[0].(string)
		}
		sessionID, err := h.deps.ExecManager.OpenSession(ctxName, ns, pod, container, shell)
		if err != nil {
			return errorJSON(err.Error())
		}
		return marshalJSON(map[string]any{"sessionId": sessionID})

	case "event.subscribe":
		if !h.perms.AllowsEvents() {
			return errorJSON("method not available: " + method)
		}
		eventName, _ := req["event"].(string)
		if h.eventCh != nil && h.deps.On != nil {
			unsub := h.deps.On(eventName, func(payloadJSON []byte) {
				select {
				case h.eventCh <- eventPayload{Name: eventName, Data: payloadJSON}:
				default:
				}
			})
			h.cleanupFns = append(h.cleanupFns, unsub)
		}
		return marshalJSON(map[string]any{"ok": true})

	case "storage.get":
		if !h.perms.AllowsStorage() {
			return errorJSON("method not available: storage.get")
		}
		if h.storage == nil {
			return errorJSON("storage not available")
		}
		key, _ := req["key"].(string)
		val, ok := h.storage.Get(key)
		if !ok {
			return marshalJSON(map[string]any{"value": nil, "found": false})
		}
		return marshalJSON(map[string]any{"value": val, "found": true})

	case "storage.set":
		if !h.perms.AllowsStorage() {
			return errorJSON("method not available: storage.set")
		}
		if h.storage == nil {
			return errorJSON("storage not available")
		}
		key, _ := req["key"].(string)
		value, _ := req["value"].(string)
		h.storage.Set(key, value)
		return marshalJSON(map[string]any{"ok": true})

	case "storage.delete":
		if !h.perms.AllowsStorage() {
			return errorJSON("method not available: storage.delete")
		}
		if h.storage == nil {
			return errorJSON("storage not available")
		}
		key, _ := req["key"].(string)
		h.storage.Delete(key)
		return marshalJSON(map[string]any{"ok": true})

	case "storage.list":
		if !h.perms.AllowsStorage() {
			return errorJSON("method not available: storage.list")
		}
		if h.storage == nil {
			return errorJSON("storage not available")
		}
		return marshalJSON(map[string]any{"keys": h.storage.List()})

	case "settings.get":
		if !h.perms.AllowsStorage() {
			return errorJSON("method not available: settings.get")
		}
		val, found := h.storage.Get("settings")
		if !found {
			return marshalJSON(map[string]any{"value": "{}", "found": false})
		}
		return marshalJSON(map[string]any{"value": val, "found": true})

	case "settings.set":
		if !h.perms.AllowsStorage() {
			return errorJSON("method not available: settings.set")
		}
		value, _ := req["value"].(string)
		if value == "" {
			return errorJSON("missing 'value'")
		}
		h.storage.Set("settings", value)
		return marshalJSON(map[string]any{"ok": true})

	case "register_template":
		gvr, _ := req["gvr"].(string)
		name, _ := req["name"].(string)
		description, _ := req["description"].(string)
		content, _ := req["content"].(string)
		if h.deps.TemplateRegistry != nil {
			h.deps.TemplateRegistry.RegisterPlugin(h.pluginName, resource.Template{
				GVR:         gvr,
				Name:        name,
				Description: description,
				Content:     content,
				Source:      "plugin:" + h.pluginName,
			})
		}
		return marshalJSON(map[string]any{"ok": true})

	default:
		return errorJSON("method not available: " + method)
	}
}


func errorJSON(msg string) []byte {
	b, _ := json.Marshal(map[string]string{"error": msg})
	return b
}

func marshalJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}
