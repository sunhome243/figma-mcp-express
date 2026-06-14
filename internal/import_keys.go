package internal

import "sync"

type libraryCatalogKeyIndex struct {
	mu    sync.RWMutex
	byKey map[string]string
}

var libraryCatalogKeys = &libraryCatalogKeyIndex{byKey: map[string]string{}}

func rememberLibraryCatalogKeys(byKey map[string]any) {
	if len(byKey) == 0 {
		return
	}

	libraryCatalogKeys.mu.Lock()
	defer libraryCatalogKeys.mu.Unlock()
	for key, raw := range byKey {
		entry, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		assetType, _ := entry["type"].(string)
		switch assetType {
		case "COMPONENT", "COMPONENT_SET", "STYLE":
			libraryCatalogKeys.byKey[key] = assetType
		}
	}
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
	if t != "import_component_by_key" {
		return
	}
	params, ok := op["params"].(map[string]interface{})
	if !ok {
		return
	}
	prepareImportComponentByKeyParams(params)
}
