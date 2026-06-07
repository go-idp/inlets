package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

// Override is an in-memory set of patches layered over a *FileConfig.
// Patches are stored as JSON-pointer-like paths (e.g. "clients[0].clientSecret")
// and applied on top of an immutable base at read time.
//
// Design notes (see AGENTS.md 2024-12-19 concurrency lesson):
//   - Apply() does NOT mutate its argument. It deep-copies via JSON
//     round-trip and writes the patches into the copy.
//   - Reads take a brief RLock, copy, return.
//   - Writes take a Lock.
//   - Patch paths that resolve to a missing field fail-fast at Set time.
type Override struct {
	mu      sync.RWMutex
	patches map[string]json.RawMessage
}

// NewOverride creates an empty Override.
func NewOverride() *Override {
	return &Override{patches: make(map[string]json.RawMessage)}
}

// OverrideEntry is the JSON-friendly view of a single patch.
type OverrideEntry struct {
	Path  string          `json:"path"`
	Value json.RawMessage `json:"value"`
}

// Set registers a patch. The path must resolve to a settable leaf in
// FileConfig; otherwise an error is returned. Values are stored as their
// JSON representation so re-marshaling is stable.
func (o *Override) Set(path string, value any) error {
	if o == nil {
		return errors.New("override: nil")
	}
	if path == "" {
		return errors.New("override: empty path")
	}
	// Validate that the path is settable against a zero-valued FileConfig.
	zero := &FileConfig{}
	if err := applyOne(zero, path, value); err != nil {
		return err
	}
	// Normalize value via JSON so it round-trips.
	raw, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("override: marshal value: %w", err)
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.patches == nil {
		o.patches = make(map[string]json.RawMessage)
	}
	o.patches[path] = raw
	return nil
}

// Get returns the raw JSON value of a patch.
func (o *Override) Get(path string) (json.RawMessage, bool) {
	if o == nil {
		return nil, false
	}
	o.mu.RLock()
	defer o.mu.RUnlock()
	v, ok := o.patches[path]
	return v, ok
}

// Delete removes a patch.
func (o *Override) Delete(path string) {
	if o == nil {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	delete(o.patches, path)
}

// ClearAll removes every patch.
func (o *Override) ClearAll() {
	if o == nil {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	o.patches = make(map[string]json.RawMessage)
}

// List returns a copy of the current patches.
func (o *Override) List() []OverrideEntry {
	if o == nil {
		return nil
	}
	o.mu.RLock()
	defer o.mu.RUnlock()
	out := make([]OverrideEntry, 0, len(o.patches))
	for p, v := range o.patches {
		out = append(out, OverrideEntry{Path: p, Value: v})
	}
	return out
}

// Size returns the number of active patches.
func (o *Override) Size() int {
	if o == nil {
		return 0
	}
	o.mu.RLock()
	defer o.mu.RUnlock()
	return len(o.patches)
}

// Apply returns a deep copy of `base` with all patches applied. The
// argument is never mutated.
func (o *Override) Apply(base *FileConfig) *FileConfig {
	if o == nil || base == nil {
		return base
	}
	o.mu.RLock()
	defer o.mu.RUnlock()
	if len(o.patches) == 0 {
		return base
	}
	// Work on a generic JSON tree because reflection over our nested
	// pointer types is brittle. The trade-off is the deep copy cost,
	// which is negligible for config files.
	raw, err := json.Marshal(base)
	if err != nil {
		return base
	}
	var tree any
	if err := json.Unmarshal(raw, &tree); err != nil {
		return base
	}
	for path, value := range o.patches {
		var v any
		if err := json.Unmarshal(value, &v); err != nil {
			continue
		}
		tree = setJSONPath(tree, parsePath(path), v)
	}
	out, err := marshalBack(tree)
	if err != nil {
		return base
	}
	return out
}

// applyOne validates that path can be set against a zero FileConfig by
// actually performing the apply on a temporary copy.
func applyOne(base *FileConfig, path string, value any) error {
	o := &Override{patches: map[string]json.RawMessage{}}
	raw, _ := json.Marshal(value)
	o.patches[path] = raw
	// Apply returns a copy; verify it didn't error out by checking that
	// the resulting tree is non-nil. We don't have a separate validation
	// path — setJSONPath always returns a tree even for unknown segments
	// (it creates the path), so to be strict we walk the segments and
	// check that each field is a known struct field of FileConfig.
	segs := parsePath(path)
	if !pathResolves(segs) {
		return fmt.Errorf("override: path %q does not resolve to a known field of FileConfig", path)
	}
	return nil
}

// pathResolves walks the segments and checks that the path matches the
// real FileConfig structure. Returns false for unknown fields.
func pathResolves(segs []string) bool {
	// Field navigation map. We model the FileConfig schema explicitly
	// here to avoid reflection on every Set.
	keys := map[string]map[string]bool{
		"": {
			"domain": true, "port": true, "tcpPort": true, "secure": true,
			"token": true, "clients": true, "notification": true,
			"bandwidthLimits": true, "publicHTTPNoAuth": true, "admin": true,
		},
		"clients[*]": {
			"clientId": true, "clientSecret": true, "config": true,
			"bandwidthLimit": true, "tunnels": true,
		},
		"clients[*].tunnels[*]": {
			"name": true, "type": true, "upstream": true, "subDomain": true,
			"remotePort": true, "auth": true, "auths": true,
		},
		"notification": {
			"provider": true, "url": true, "interval": true, "alert": true,
		},
		"notification.alert": {
			"provider": true, "url": true, "interval": true,
		},
		"bandwidthLimits": {
			"global": true, "clients": true,
		},
		"publicHTTPNoAuth": {
			"timeout": true, "warnLead": true,
		},
		"admin": {
			"enabled": true, "listen": true, "database": true, "runtime": true, "ui": true,
		},
		"admin.database": {"path": true},
		"admin.runtime":  {"pidFile": true, "snapshotInterval": true},
		"admin.ui":       {"basePath": true},
	}
	// Walk segments; at each step, look up the parent prefix in `keys`
	// and verify the next field name is one of its allowed children.
	// The "parent prefix" uses [*] for any numeric segments.
	tail := ""
	for _, s := range segs {
		if s == "" {
			return false
		}
		isIndex := true
		for _, c := range s {
			if c < '0' || c > '9' {
				isIndex = false
				break
			}
		}
		if isIndex {
			tail = tail + "[" + s + "]"
			continue
		}
		// Look up children of `tail`. We translate the bracketed form
		// to [*] form for the lookup.
		parent := shorten(tail)
		allowed, ok := keys[parent]
		if !ok {
			return false
		}
		if !allowed[s] {
			return false
		}
		if tail == "" {
			tail = s
		} else {
			tail = tail + "." + s
		}
	}
	return true
}

// shorten converts "clients[0].tunnels[1]" to "clients[*].tunnels[*]".
func shorten(p string) string {
	var sb strings.Builder
	i := 0
	for i < len(p) {
		if p[i] == '[' {
			sb.WriteString("[*]")
			for i < len(p) && p[i] != ']' {
				i++
			}
			i++ // skip ']'
		} else {
			sb.WriteByte(p[i])
			i++
		}
	}
	return sb.String()
}

// setJSONPath mutates `tree` (a generic any from json.Unmarshal) at the
// given segments, returning the new tree. Missing intermediate objects
// are created.
func setJSONPath(tree any, segs []string, value any) any {
	if len(segs) == 0 {
		return value
	}
	head, tail := segs[0], segs[1:]
	// Index segment like "0" → array index.
	if idx, err := parseIndex(head); err == nil {
		arr, ok := tree.([]any)
		if !ok {
			arr = []any{}
		}
		for len(arr) <= idx {
			arr = append(arr, nil)
		}
		arr[idx] = setJSONPath(arr[idx], tail, value)
		return arr
	}
	// Object segment.
	obj, ok := tree.(map[string]any)
	if !ok {
		obj = map[string]any{}
	}
	obj[head] = setJSONPath(obj[head], tail, value)
	return obj
}

func parseIndex(s string) (int, error) {
	if s == "" {
		return 0, errors.New("empty")
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, errors.New("not a number")
		}
	}
	n := 0
	for _, c := range s {
		n = n*10 + int(c-'0')
	}
	return n, nil
}

func marshalBack(tree any) (*FileConfig, error) {
	raw, err := json.Marshal(tree)
	if err != nil {
		return nil, err
	}
	var out FileConfig
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// parsePath converts "a.b[0].c" into ["a", "b", "0", "c"].
func parsePath(path string) []string {
	out := []string{}
	i := 0
	for i < len(path) {
		switch path[i] {
		case '.':
			i++
		case '[':
			j := i + 1
			for j < len(path) && path[j] != ']' {
				j++
			}
			if j >= len(path) {
				out = append(out, path[i+1:])
				return out
			}
			out = append(out, path[i+1:j])
			i = j + 1
		default:
			j := i
			for j < len(path) && path[j] != '.' && path[j] != '[' {
				j++
			}
			out = append(out, path[i:j])
			i = j
		}
	}
	return out
}

// IsDurationField returns true if the leaf at `path` is expected to be a
// duration string. Used by the admin UI to format input correctly.
func IsDurationField(path string) bool {
	short := shorten(path)
	return short == "publicHTTPNoAuth.timeout" || short == "publicHTTPNoAuth.warnLead"
}

// keep time.Duration import alive for future field-aware types.
var _ = time.Second
