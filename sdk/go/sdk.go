//go:build wasip1

// Package sdk provides a high-level API for klados plugin authors.
// Import this package in your plugin's main package to register enrichers
// and interact with the host via K8s, Storage, Log, and event subscriptions.
//
// Example:
//
//	import sdk "github.com/Vilsol/klados-plugin-sdk"
//
//	func init() {
//	    sdk.RegisterEnricher("core.v1.pods", func(obj map[string]any) map[string]any {
//	        obj["status"].(map[string]any)["myField"] = "hello"
//	        return obj
//	    })
//	}
package sdk

import (
	"encoding/json"
	"unsafe"

	"github.com/Vilsol/klados-plugin-sdk/internal"
)

// enricherFn is the type for enricher functions.
type enricherFn func(obj map[string]any) map[string]any

var enrichers = map[string]enricherFn{}

// RegisterEnricher registers a function that enriches objects for the given GVR.
// The function receives the full unstructured k8s object and should return it (modified).
func RegisterEnricher(gvr string, fn func(obj map[string]any) map[string]any) {
	enrichers[gvr] = fn
}

var commandHandlers = map[string]func(){}

// OnCommand registers a handler for the given command ID.
func OnCommand(id string, fn func()) {
	commandHandlers[id] = fn
}

// DispatchCommand delivers a command invocation to the registered handler.
func DispatchCommand(idPtr, idLen uint32) {
	id := string(ReadGuestBytes(idPtr, idLen))
	if fn, ok := commandHandlers[id]; ok {
		fn()
	}
}

var eventHandlers = map[string][]func([]byte){}

// OnEvent registers a callback for a specific event type.
func OnEvent(eventType string, fn func(payload []byte)) {
	eventHandlers[eventType] = append(eventHandlers[eventType], fn)
}

// --- K8s client ---

var K8s = &k8sClient{}

type k8sClient struct{}

func (k *k8sClient) List(gvr, ns string) ([]map[string]any, error) {
	req, _ := json.Marshal(map[string]any{"gvr": gvr, "namespace": ns})
	resp := internal.Call("k8s.list", req)
	var result struct {
		Items []map[string]any `json:"items"`
		Error string           `json:"error"`
	}
	if err := unmarshalResponse(resp, &result); err != nil {
		return nil, err
	}
	return result.Items, nil
}

func (k *k8sClient) Get(gvr, ns, name string) (map[string]any, error) {
	req, _ := json.Marshal(map[string]any{"gvr": gvr, "namespace": ns, "name": name})
	resp := internal.Call("k8s.get", req)
	var result struct {
		Object map[string]any `json:"object"`
		Error  string         `json:"error"`
	}
	if err := unmarshalResponse(resp, &result); err != nil {
		return nil, err
	}
	return result.Object, nil
}

func (k *k8sClient) Create(gvr, ns string, obj map[string]any) (map[string]any, error) {
	req, _ := json.Marshal(map[string]any{"gvr": gvr, "namespace": ns, "object": obj})
	resp := internal.Call("k8s.create", req)
	var result struct {
		Object map[string]any `json:"object"`
		Error  string         `json:"error"`
	}
	if err := unmarshalResponse(resp, &result); err != nil {
		return nil, err
	}
	return result.Object, nil
}

func (k *k8sClient) Update(gvr, ns string, obj map[string]any) (map[string]any, error) {
	req, _ := json.Marshal(map[string]any{"gvr": gvr, "namespace": ns, "object": obj})
	resp := internal.Call("k8s.update", req)
	var result struct {
		Object map[string]any `json:"object"`
		Error  string         `json:"error"`
	}
	if err := unmarshalResponse(resp, &result); err != nil {
		return nil, err
	}
	return result.Object, nil
}

func (k *k8sClient) Delete(gvr, ns, name string) error {
	req, _ := json.Marshal(map[string]any{"gvr": gvr, "namespace": ns, "name": name})
	resp := internal.Call("k8s.delete", req)
	var result struct {
		Error string `json:"error"`
	}
	return unmarshalResponse(resp, &result)
}

// --- Storage client ---

var Storage = &storageClient{}

type storageClient struct{}

func (s *storageClient) Get(key string) (string, bool, error) {
	req, _ := json.Marshal(map[string]any{"key": key})
	resp := internal.Call("storage.get", req)
	var result struct {
		Value *string `json:"value"`
		Found bool    `json:"found"`
		Error string  `json:"error"`
	}
	if err := unmarshalResponse(resp, &result); err != nil {
		return "", false, err
	}
	if result.Value == nil {
		return "", result.Found, nil
	}
	return *result.Value, result.Found, nil
}

func (s *storageClient) Set(key, value string) error {
	req, _ := json.Marshal(map[string]any{"key": key, "value": value})
	resp := internal.Call("storage.set", req)
	var result struct {
		Error string `json:"error"`
	}
	return unmarshalResponse(resp, &result)
}

func (s *storageClient) Delete(key string) error {
	req, _ := json.Marshal(map[string]any{"key": key})
	resp := internal.Call("storage.delete", req)
	var result struct {
		Error string `json:"error"`
	}
	return unmarshalResponse(resp, &result)
}

func (s *storageClient) List() ([]string, error) {
	resp := internal.Call("storage.list", nil)
	var result struct {
		Keys  []string `json:"keys"`
		Error string   `json:"error"`
	}
	if err := unmarshalResponse(resp, &result); err != nil {
		return nil, err
	}
	return result.Keys, nil
}

// --- Logging ---

var Log = &logger{}

type logger struct{}

func (l *logger) Debug(msg string) { internal.Log(0, msg) }
func (l *logger) Info(msg string)  { internal.Log(1, msg) }
func (l *logger) Warn(msg string)  { internal.Log(2, msg) }
func (l *logger) Error(msg string) { internal.Log(3, msg) }

// --- Shared helper for reading guest memory ---

// ReadGuestBytes copies a slice from guest linear memory into a new Go slice.
func ReadGuestBytes(ptr, length uint32) []byte {
	if length == 0 {
		return nil
	}
	src := unsafe.Slice((*byte)(unsafe.Pointer(uintptr(ptr))), int(length))
	out := make([]byte, length)
	copy(out, src)
	return out
}

// WriteGuestBytes writes data to the given guest pointer.
func WriteGuestBytes(ptr uint32, data []byte) {
	if len(data) == 0 {
		return
	}
	dst := unsafe.Slice((*byte)(unsafe.Pointer(uintptr(ptr))), len(data))
	copy(dst, data)
}

// EnrichDispatch implements the plugin_enrich logic for use by both Go and TinyGo builds.
// gvrPtr/gvrLen: GVR string in guest memory. objPtr/objLen: JSON object in guest memory.
// Returns packed (outputPtr << 32 | outputLen).
func EnrichDispatch(allocFn func(uint32) uint32, gvrPtr, gvrLen, objPtr, objLen uint32) uint64 {
	if objLen == 0 {
		return 0
	}
	gvr := string(ReadGuestBytes(gvrPtr, gvrLen))
	fn, ok := enrichers[gvr]
	if !ok {
		// No enricher — echo unchanged
		out := allocFn(objLen)
		WriteGuestBytes(out, ReadGuestBytes(objPtr, objLen))
		return uint64(out)<<32 | uint64(objLen)
	}

	var obj map[string]any
	if err := json.Unmarshal(ReadGuestBytes(objPtr, objLen), &obj); err != nil {
		return 0
	}
	enriched := fn(obj)
	outBytes, err := json.Marshal(enriched)
	if err != nil {
		return 0
	}

	out := allocFn(uint32(len(outBytes)))
	WriteGuestBytes(out, outBytes)
	return uint64(out)<<32 | uint64(len(outBytes))
}

// DispatchOnEvent delivers an event to all registered handlers.
func DispatchOnEvent(typePtr, typeLen, payloadPtr, payloadLen uint32) {
	eventType := string(ReadGuestBytes(typePtr, typeLen))
	payload := ReadGuestBytes(payloadPtr, payloadLen)
	for _, fn := range eventHandlers[eventType] {
		fn(payload)
	}
}

func unmarshalResponse(resp []byte, out any) error {
	if len(resp) == 0 {
		return nil
	}
	if err := json.Unmarshal(resp, out); err != nil {
		return err
	}
	var raw map[string]any
	if err := json.Unmarshal(resp, &raw); err == nil {
		if errMsg, ok := raw["error"].(string); ok && errMsg != "" {
			return &hostError{msg: errMsg}
		}
	}
	return nil
}

type hostError struct{ msg string }

func (e *hostError) Error() string { return e.msg }
