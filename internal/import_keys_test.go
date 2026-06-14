package internal

import (
	"fmt"
	"testing"
)

func resetLibraryCatalogIndexForTest() {
	libraryCatalogKeys.mu.Lock()
	defer libraryCatalogKeys.mu.Unlock()
	libraryCatalogKeys.byKey = map[string]string{}
}

func TestRememberLibraryCatalogKeysStoresOnlyComponentSetHints(t *testing.T) {
	resetLibraryCatalogIndexForTest()

	rememberLibraryCatalogKeys(map[string]any{
		"component-key":     map[string]any{"type": "COMPONENT"},
		"component-set-key": map[string]any{"type": "COMPONENT_SET"},
		"style-key":         map[string]any{"type": "STYLE"},
	})

	if assetType, ok := lookupLibraryCatalogAssetType("component-set-key"); !ok || assetType != "COMPONENT_SET" {
		t.Fatalf("component set hint = %q, %v; want COMPONENT_SET, true", assetType, ok)
	}
	if assetType, ok := lookupLibraryCatalogAssetType("component-key"); ok || assetType != "" {
		t.Fatalf("component key should not be cached, got %q, %v", assetType, ok)
	}
	if assetType, ok := lookupLibraryCatalogAssetType("style-key"); ok || assetType != "" {
		t.Fatalf("style key should not be cached, got %q, %v", assetType, ok)
	}
}

func TestRememberLibraryCatalogKeysCapsGrowth(t *testing.T) {
	resetLibraryCatalogIndexForTest()

	oversized := make(map[string]any, libraryCatalogKeyIndexMax+100)
	for i := 0; i < libraryCatalogKeyIndexMax+100; i++ {
		oversized[fmt.Sprintf("component-set-%05d", i)] = map[string]any{"type": "COMPONENT_SET"}
	}
	rememberLibraryCatalogKeys(oversized)

	libraryCatalogKeys.mu.RLock()
	got := len(libraryCatalogKeys.byKey)
	libraryCatalogKeys.mu.RUnlock()
	if got != libraryCatalogKeyIndexMax {
		t.Fatalf("cache size = %d, want cap %d", got, libraryCatalogKeyIndexMax)
	}
}
