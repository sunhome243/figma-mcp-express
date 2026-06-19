package internal

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
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

func TestResolveImagePath(t *testing.T) {
	// write a temp image file
	dir := t.TempDir()
	imgData := []byte("fake-image-bytes")
	imgPath := filepath.Join(dir, "photo.jpg")
	if err := os.WriteFile(imgPath, imgData, 0o644); err != nil {
		t.Fatal(err)
	}
	want := base64.StdEncoding.EncodeToString(imgData)

	t.Run("absolute path converted to imageData", func(t *testing.T) {
		params := map[string]interface{}{"imagePath": imgPath}
		resolveImagePath(params)
		if got, ok := params["imageData"].(string); !ok || got != want {
			t.Fatalf("imageData = %q; want %q", got, want)
		}
		if _, still := params["imagePath"]; still {
			t.Fatal("imagePath should be removed after conversion")
		}
	})

	t.Run("relative path resolved against cwd", func(t *testing.T) {
		// change into the temp dir so relative path works
		orig, _ := os.Getwd()
		if err := os.Chdir(dir); err != nil {
			t.Fatal(err)
		}
		defer os.Chdir(orig) //nolint:errcheck
		params := map[string]interface{}{"imagePath": "photo.jpg"}
		resolveImagePath(params)
		if got, ok := params["imageData"].(string); !ok || got != want {
			t.Fatalf("imageData = %q; want %q", got, want)
		}
	})

	t.Run("imagePath wins over existing imageData and imageUrl", func(t *testing.T) {
		params := map[string]interface{}{"imagePath": imgPath, "imageData": "existing-data", "imageUrl": "https://example.com/remote.png"}
		resolveImagePath(params)
		if got := params["imageData"].(string); got != want {
			t.Fatalf("imageData = %q; want %q", got, want)
		}
		if _, still := params["imageUrl"]; still {
			t.Fatal("imageUrl should be removed when imagePath is resolved")
		}
	})

	t.Run("imageData wins over imageUrl when no imagePath", func(t *testing.T) {
		params := map[string]interface{}{"imageData": "existing-data", "imageUrl": "https://example.com/remote.png"}
		resolveImagePath(params)
		if got := params["imageData"].(string); got != "existing-data" {
			t.Fatalf("imageData = %q; want existing-data", got)
		}
		if _, still := params["imageUrl"]; still {
			t.Fatal("imageUrl should be removed when imageData is present")
		}
	})

	t.Run("missing imagePath — no-op", func(t *testing.T) {
		params := map[string]interface{}{}
		resolveImagePath(params) // must not panic
		if _, ok := params["imageData"]; ok {
			t.Fatal("imageData should not be set when imagePath absent")
		}
	})

	t.Run("nonexistent file — no-op (plugin reports error)", func(t *testing.T) {
		params := map[string]interface{}{"imagePath": "/nonexistent/photo.jpg"}
		resolveImagePath(params)
		if _, ok := params["imageData"]; ok {
			t.Fatal("imageData should not be set when file missing")
		}
	})
}

func TestPrepareBatchImportParamsHandlesImportImage(t *testing.T) {
	dir := t.TempDir()
	imgData := []byte("pixels")
	imgPath := filepath.Join(dir, "img.jpg")
	if err := os.WriteFile(imgPath, imgData, 0o644); err != nil {
		t.Fatal(err)
	}
	want := base64.StdEncoding.EncodeToString(imgData)

	ops := []interface{}{
		map[string]interface{}{
			"type":   "import_image",
			"params": map[string]interface{}{"imagePath": imgPath, "width": float64(100)},
		},
	}
	prepareBatchImportParams(ops)
	params := ops[0].(map[string]interface{})["params"].(map[string]interface{})
	if got, ok := params["imageData"].(string); !ok || got != want {
		t.Fatalf("batch import_image: imageData = %q; want %q", got, want)
	}
	if _, still := params["imagePath"]; still {
		t.Fatal("imagePath should be removed after prepareBatchImportParams")
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
