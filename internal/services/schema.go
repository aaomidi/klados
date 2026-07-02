package services

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Vilsol/slox"
	"github.com/adrg/xdg"

	"github.com/Vilsol/klados/internal/resource"
)

type SchemaService struct {
	appSvc *AppService
	ctx    context.Context
}

func NewSchemaService(appSvc *AppService) *SchemaService {
	return &SchemaService{appSvc: appSvc}
}

func (s *SchemaService) Startup(ctx context.Context) error {
	s.ctx = ctx
	return nil
}

func (s *SchemaService) Shutdown() error {
	return nil
}

// GetSchema returns the JSON Schema for the given GVR, extracted from the cluster's OpenAPI v3 spec.
// The schema is cached to disk keyed by server URL + server version to survive restarts.
// kind must be the PascalCase resource Kind (e.g. "Deployment") used to locate the schema in OpenAPI.
func (s *SchemaService) GetSchema(contextName, gvr, kind string) (map[string]any, error) {
	conn, err := s.appSvc.ClusterManager().GetConnection(contextName)
	if err != nil {
		return nil, err
	}

	// Derive kind from GVR via discovery when not provided.
	if kind == "" {
		resources, err := s.appSvc.ClusterManager().DiscoverResources(contextName)
		if err != nil && len(resources) == 0 {
			return nil, fmt.Errorf("discover resources: %w", err)
		}
		if err != nil {
			slox.Warn(s.ctx, "using partial discovery results", "context", contextName, "error", err)
		}
		for _, r := range resources {
			if r.GVR == gvr {
				kind = r.Kind
				break
			}
		}
	}

	sv, err := conn.Clientset.Discovery().ServerVersion()
	if err != nil {
		return nil, fmt.Errorf("getting server version: %w", err)
	}

	// Use server URL as a stable cluster identifier (more stable than context name).
	serverURL := conn.Config.Host
	urlHash := fmt.Sprintf("%x", sha256.Sum256([]byte(serverURL)))[:12]
	safeGVR := strings.ReplaceAll(gvr, "/", "_")
	cacheKey := fmt.Sprintf("%s-%s-%s", urlHash, sv.GitVersion, safeGVR)
	cachePath := filepath.Join(xdg.CacheHome, "klados", "schemas", cacheKey+".json")

	if data, err := os.ReadFile(cachePath); err == nil {
		var schema map[string]any
		if json.Unmarshal(data, &schema) == nil {
			return schema, nil
		}
	}

	parsed, err := resource.ParseGVR(gvr)
	if err != nil {
		return nil, err
	}

	var apiPath string
	if parsed.Group == "" {
		apiPath = fmt.Sprintf("/openapi/v3/api/%s", parsed.Version)
	} else {
		apiPath = fmt.Sprintf("/openapi/v3/apis/%s/%s", parsed.Group, parsed.Version)
	}

	body, err := conn.Clientset.Discovery().RESTClient().Get().
		AbsPath(apiPath).
		DoRaw(s.ctx)
	if err != nil {
		return nil, fmt.Errorf("fetching schema from %s: %w", apiPath, err)
	}

	var fullDoc map[string]any
	if err := json.Unmarshal(body, &fullDoc); err != nil {
		return nil, fmt.Errorf("parsing OpenAPI doc: %w", err)
	}

	schema := extractResourceSchema(fullDoc, parsed.Group, parsed.Version, kind)
	if schema == nil {
		// Fallback: return full doc so caller still has something
		schema = fullDoc
	} else {
		bundleSchemaRefs(schema, fullDoc)
	}

	data, _ := json.Marshal(schema)
	if data != nil {
		if err := os.MkdirAll(filepath.Dir(cachePath), 0o755); err == nil {
			_ = os.WriteFile(cachePath, data, 0o644)
		}
	}

	return schema, nil
}

const openAPIRefPrefix = "#/components/schemas/"
const jsonSchemaRefPrefix = "#/definitions/"

// bundleSchemaRefs collects all transitively-referenced schemas from the OpenAPI
// document's components/schemas and attaches them as "definitions" on the root schema.
// It rewrites $ref pointers from OpenAPI format (#/components/schemas/X) to
// JSON Schema Draft-07 format (#/definitions/X) and unwraps single-element
// allOf wrappers (allOf: [{$ref: "..."}]) to plain $ref, since json-schema-library's
// Draft07 cannot resolve $ref through allOf.
func bundleSchemaRefs(schema, fullDoc map[string]any) {
	components, ok := fullDoc["components"].(map[string]any)
	if !ok {
		return
	}
	allSchemas, ok := components["schemas"].(map[string]any)
	if !ok {
		return
	}

	defs := make(map[string]any)
	visited := make(map[string]bool)

	// Collect all $ref targets transitively, rewriting refs and unwrapping allOf.
	var collect func(node any)
	collect = func(node any) {
		switch v := node.(type) {
		case map[string]any:
			// Unwrap allOf: [{"$ref": "..."}] → {"$ref": "..."} with sibling fields preserved.
			unwrapAllOfRef(v)

			if ref, ok := v["$ref"].(string); ok {
				if name, found := strings.CutPrefix(ref, openAPIRefPrefix); found {
					v["$ref"] = jsonSchemaRefPrefix + name
					if !visited[name] {
						visited[name] = true
						if def, ok := allSchemas[name].(map[string]any); ok {
							defs[name] = def
							collect(def)
						}
					}
				}
			}
			for _, child := range v {
				collect(child)
			}
		case []any:
			for _, item := range v {
				collect(item)
			}
		}
	}

	collect(schema)

	if len(defs) > 0 {
		schema["definitions"] = defs
	}
}

// unwrapAllOfRef converts allOf: [{"$ref": "..."}] into a direct $ref on the parent map.
// Kubernetes OpenAPI wraps $ref in allOf alongside "default" and "description" fields.
// json-schema-library Draft07 can't resolve $ref through allOf, so we flatten it.
func unwrapAllOfRef(m map[string]any) {
	allOf, ok := m["allOf"].([]any)
	if !ok || len(allOf) != 1 {
		return
	}
	entry, ok := allOf[0].(map[string]any)
	if !ok {
		return
	}
	ref, ok := entry["$ref"].(string)
	if !ok {
		return
	}
	// Move $ref up, remove allOf.
	m["$ref"] = ref
	delete(m, "allOf")
}

// extractResourceSchema finds the JSON Schema for a specific resource Kind within an OpenAPI v3 document.
// Kubernetes uses the x-kubernetes-group-version-kind extension to tag schemas.
func extractResourceSchema(doc map[string]any, group, version, kind string) map[string]any {
	if kind == "" {
		return nil
	}

	components, ok := doc["components"].(map[string]any)
	if !ok {
		return nil
	}
	schemas, ok := components["schemas"].(map[string]any)
	if !ok {
		return nil
	}

	wantGroup := group
	if wantGroup == "" {
		wantGroup = ""
	}

	for _, v := range schemas {
		schema, ok := v.(map[string]any)
		if !ok {
			continue
		}
		gvkList, ok := schema["x-kubernetes-group-version-kind"].([]any)
		if !ok {
			continue
		}
		for _, gvkRaw := range gvkList {
			gvk, ok := gvkRaw.(map[string]any)
			if !ok {
				continue
			}
			if gvk["group"] == wantGroup && gvk["version"] == version && gvk["kind"] == kind {
				return schema
			}
		}
	}
	return nil
}
