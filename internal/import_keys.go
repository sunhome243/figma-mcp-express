package internal

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

type libraryCatalogKeyIndex struct {
	mu    sync.RWMutex
	byKey map[string]string
}

var libraryCatalogKeys = &libraryCatalogKeyIndex{byKey: map[string]string{}}

const libraryCatalogKeyIndexMax = 10000

func rememberLibraryCatalogKeys(byKey map[string]any) {
	if len(byKey) == 0 {
		return
	}

	hints := make(map[string]string)
	for key, raw := range byKey {
		entry, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		assetType, _ := entry["type"].(string)
		if assetType == "COMPONENT_SET" {
			hints[key] = assetType
		}
	}
	if len(hints) == 0 {
		return
	}

	libraryCatalogKeys.mu.Lock()
	defer libraryCatalogKeys.mu.Unlock()

	if len(hints) >= libraryCatalogKeyIndexMax {
		libraryCatalogKeys.byKey = limitLibraryCatalogKeyIndex(hints, libraryCatalogKeyIndexMax)
		return
	}
	additions := 0
	for key := range hints {
		if _, ok := libraryCatalogKeys.byKey[key]; !ok {
			additions++
		}
	}
	if len(libraryCatalogKeys.byKey)+additions > libraryCatalogKeyIndexMax {
		libraryCatalogKeys.byKey = map[string]string{}
	}
	for key, assetType := range hints {
		libraryCatalogKeys.byKey[key] = assetType
	}
}

func limitLibraryCatalogKeyIndex(in map[string]string, max int) map[string]string {
	keys := make([]string, 0, len(in))
	for key := range in {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	if len(keys) > max {
		keys = keys[:max]
	}
	out := make(map[string]string, len(keys))
	for _, key := range keys {
		out[key] = in[key]
	}
	return out
}

func lookupLibraryCatalogAssetType(key string) (string, bool) {
	libraryCatalogKeys.mu.RLock()
	defer libraryCatalogKeys.mu.RUnlock()
	assetType, ok := libraryCatalogKeys.byKey[key]
	return assetType, ok
}

func prepareImportComponentByKeyParams(params map[string]interface{}) {
	if params == nil {
		return
	}
	key, _ := params["key"].(string)
	if key == "" {
		return
	}
	if assetType, _ := params["assetType"].(string); assetType != "" {
		return
	}
	assetType, ok := lookupLibraryCatalogAssetType(key)
	if !ok {
		return
	}
	switch assetType {
	case "COMPONENT", "COMPONENT_SET":
		params["assetType"] = assetType
	}
}

func prepareBatchImportParams(rawOps []interface{}) {
	for _, raw := range rawOps {
		op, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		prepareBatchImportOpParams(op)
	}
}

func prepareBatchImportOpParams(op map[string]interface{}) {
	t, _ := op["type"].(string)
	if t == "map" {
		if do, ok := op["do"].(map[string]interface{}); ok {
			prepareBatchImportOpParams(do)
		}
		return
	}
	params, ok := op["params"].(map[string]interface{})
	if !ok {
		return
	}
	switch t {
	case "import_component_by_key":
		prepareImportComponentByKeyParams(params)
	case "import_image":
		resolveImagePath(params)
	}
}

// resolveImagePath applies import_image precedence for batch params: imagePath
// resolves to imageData when readable, and local/base64 input wins over imageUrl.
// Mirrors the standalone import_image tool handler.
func resolveImagePath(params map[string]interface{}) {
	imagePath, _ := params["imagePath"].(string)
	if imagePath == "" {
		if imageData, _ := params["imageData"].(string); imageData != "" {
			delete(params, "imageUrl")
		}
		return
	}
	abs := imagePath
	if !filepath.IsAbs(abs) {
		wd, err := os.Getwd()
		if err != nil {
			return
		}
		abs = filepath.Join(wd, imagePath)
	}
	raw, err := os.ReadFile(abs)
	if err != nil {
		return // plugin will surface the missing-imageData error with context
	}
	params["imageData"] = base64.StdEncoding.EncodeToString(raw)
	delete(params, "imagePath")
	delete(params, "imageUrl")
}
