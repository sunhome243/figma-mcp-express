package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// newTestServer returns an MCPServer with all tools + prompts registered
// against an Unknown-role Node (no real Figma connection).
func newTestServer(t *testing.T) (*server.MCPServer, *Node) {
	t.Helper()
	return newTestServerWithProfile(t, "full")
}

func newTestServerWithProfile(t *testing.T, profile string) (*server.MCPServer, *Node) {
	t.Helper()
	t.Setenv("FIGMA_MCP_TOOL_PROFILE", profile)
	s := server.NewMCPServer("test", "0.0.1")
	node := NewNode("127.0.0.1", 19940, "test")
	RegisterTools(s, node)
	RegisterPrompts(s)
	return s, node
}

func newTestServerDefaultProfile(t *testing.T) (*server.MCPServer, *Node) {
	t.Helper()
	t.Setenv("FIGMA_MCP_TOOL_PROFILE", "")
	s := server.NewMCPServer("test", "0.0.1")
	node := NewNode("127.0.0.1", 19940, "test")
	RegisterTools(s, node)
	RegisterPrompts(s)
	return s, node
}

// callTool dispatches a tool call through the server's full HandleMessage path.
// With an Unknown node, every call succeeds at the MCP level but returns
// an IsError=true tool result (no Figma connection).
func callTool(t *testing.T, s *server.MCPServer, name string, args map[string]any) {
	t.Helper()
	argsJSON, _ := json.Marshal(args)
	msg := fmt.Sprintf(
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":%q,"arguments":%s}}`,
		name, argsJSON,
	)
	resp := s.HandleMessage(context.Background(), []byte(msg))
	if resp == nil {
		t.Errorf("HandleMessage returned nil for tool %q", name)
	}
}

type recordedRPCServer struct {
	server   *httptest.Server
	mu       sync.Mutex
	requests []RPCRequest
}

func newRecordedRPCServer(t *testing.T) *recordedRPCServer {
	t.Helper()
	rec := &recordedRPCServer{}
	rec.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rpc" {
			http.NotFound(w, r)
			return
		}
		var req RPCRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode RPC request: %v", err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		rec.mu.Lock()
		rec.requests = append(rec.requests, req)
		rec.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(RPCResponse{Data: map[string]any{"ok": true}})
	}))
	t.Cleanup(rec.server.Close)
	return rec
}

func (rec *recordedRPCServer) count() int {
	rec.mu.Lock()
	defer rec.mu.Unlock()
	return len(rec.requests)
}

func (rec *recordedRPCServer) lastRequest(t *testing.T) RPCRequest {
	t.Helper()
	rec.mu.Lock()
	defer rec.mu.Unlock()
	if len(rec.requests) == 0 {
		t.Fatal("expected at least one RPC request")
	}
	return rec.requests[len(rec.requests)-1]
}

func newTestServerWithRPCURL(t *testing.T, leaderURL string) (*server.MCPServer, *Node) {
	t.Helper()
	t.Setenv("FIGMA_MCP_TOOL_PROFILE", "full")
	s := server.NewMCPServer("test", "0.0.1")
	node := NewNode("127.0.0.1", 19940, "test")
	node.follower = NewFollower(leaderURL)
	RegisterTools(s, node)
	RegisterPrompts(s)
	return s, node
}

// ── Registration smoke tests ──────────────────────────────────────────────────

func TestRegisterTools_Smoke(t *testing.T) {
	s := server.NewMCPServer("test", "0.0.1")
	RegisterTools(s, NewNode("127.0.0.1", 19940, "test"))
}

func TestRegisterPrompts_Smoke(t *testing.T) {
	s := server.NewMCPServer("test", "0.0.1")
	RegisterPrompts(s)
}

// ── makeHandler ───────────────────────────────────────────────────────────────

func TestMakeHandler_UnknownNode(t *testing.T) {
	node := NewNode("127.0.0.1", 19940, "test")
	handler := makeHandler(node, "get_document", nil, nil)
	result, err := handler(context.Background(), mcp.CallToolRequest{})
	if err != nil {
		t.Fatalf("handler returned Go error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true when node has no Figma connection")
	}
}

// ── Read – no-param tools (all use makeHandler) ───────────────────────────────

func TestHandlers_NoParamReadTools(t *testing.T) {
	s, _ := newTestServer(t)
	noParamTools := []string{
		"get_document", "get_pages", "get_metadata", "get_selection",
		"get_viewport", "get_fonts", "get_styles", "get_variable_defs",
		"get_local_components", "get_annotations", "get_file_thumbnail",
		"get_selection_colors",
	}
	for _, name := range noParamTools {
		callTool(t, s, name, nil)
	}
}

// ── Read – param tools ────────────────────────────────────────────────────────

func TestHandlers_GetNode(t *testing.T) {
	s, _ := newTestServer(t)
	callTool(t, s, "get_node", map[string]any{"nodeId": "1:1"})
}

func TestHandlers_GetNodesInfo(t *testing.T) {
	s, _ := newTestServer(t)
	callTool(t, s, "get_nodes_info", map[string]any{"nodeIds": []string{"1:1", "2:2"}})
}

func TestHandlers_GetDesignContext(t *testing.T) {
	s, _ := newTestServer(t)
	// with all optional params
	callTool(t, s, "get_design_context", map[string]any{
		"depth": float64(2), "detail": "compact", "dedupe_components": true,
	})
	// with no params (defaults)
	callTool(t, s, "get_design_context", nil)
	// depth = 0 should be ignored (not passed through)
	callTool(t, s, "get_design_context", map[string]any{"depth": float64(0)})
}

func TestHandlers_SearchNodes(t *testing.T) {
	s, _ := newTestServer(t)
	// all optional params present
	callTool(t, s, "search_nodes", map[string]any{
		"query":  "button",
		"nodeId": "1:1",
		"types":  []any{"TEXT", "FRAME"},
		"limit":  float64(25),
	})
	// minimal (query only)
	callTool(t, s, "search_nodes", map[string]any{"query": "icon"})
}

func TestHandlers_ScanTextNodes(t *testing.T) {
	s, _ := newTestServer(t)
	callTool(t, s, "scan_text_nodes", map[string]any{"nodeId": "1:1"})
}

func TestHandlers_ScanNodesByTypes(t *testing.T) {
	s, _ := newTestServer(t)
	callTool(t, s, "scan_nodes_by_types", map[string]any{
		"nodeId": "1:1",
		"types":  []any{"FRAME", "COMPONENT"},
	})
}

func TestHandlers_GetReactions(t *testing.T) {
	s, _ := newTestServer(t)
	callTool(t, s, "get_reactions", map[string]any{"nodeId": "1:1"})
}

func TestHandlers_APIGapReadTools(t *testing.T) {
	s, _ := newTestServer(t)

	callTool(t, s, "get_image_by_hash", map[string]any{"hash": "abc"})
	callTool(t, s, "get_dev_resources", map[string]any{"nodeId": "1:1", "includeChildren": true})
	callTool(t, s, "resolve_variable_for_consumer", map[string]any{"variableId": "v1", "nodeId": "1:1"})
}

// ── Read – export tools ───────────────────────────────────────────────────────

func TestHandlers_GetScreenshot(t *testing.T) {
	s, _ := newTestServer(t)
	// with format + scale
	callTool(t, s, "get_screenshot", map[string]any{
		"nodeIds": []any{"1:1"},
		"format":  "PNG",
		"scale":   float64(2),
	})
	// no params (exports current selection)
	callTool(t, s, "get_screenshot", nil)
}

// TestHandlers_SaveScreenshots exercises executeSaveScreenshots + saveScreenshotItem.
// With Unknown node, node.Send fails → error captured in result JSON (no panic).
func TestHandlers_SaveScreenshots(t *testing.T) {
	s, _ := newTestServer(t)

	// single item – reaches saveScreenshotItem → node.Send fails → error result
	callTool(t, s, "save_screenshots", map[string]any{
		"items": []any{
			map[string]any{"nodeId": "1:1", "outputPath": "out/screen.png"},
		},
	})

	// multiple items with default format + scale
	callTool(t, s, "save_screenshots", map[string]any{
		"format": "SVG",
		"scale":  float64(1),
		"items": []any{
			map[string]any{"nodeId": "1:1", "outputPath": "out/a.svg"},
			map[string]any{"nodeId": "2:2", "outputPath": "out/b.svg", "format": "PNG"},
		},
	})

	// item with explicit per-item format + scale
	callTool(t, s, "save_screenshots", map[string]any{
		"items": []any{
			map[string]any{"nodeId": "3:3", "outputPath": "out/c.jpg", "format": "JPG", "scale": float64(2)},
		},
	})
}

// ── Write – create tools ──────────────────────────────────────────────────────

func TestHandlers_WriteCreateTools(t *testing.T) {
	s, _ := newTestServer(t)

	callTool(t, s, "create_frame", map[string]any{
		"width": float64(100), "height": float64(100), "name": "Card",
		"layoutMode": "VERTICAL", "parentId": "1:1",
	})
	callTool(t, s, "create_frame", map[string]any{}) // minimal

	callTool(t, s, "create_rectangle", map[string]any{"fillColor": "#FF5733", "cornerRadius": float64(8)})
	callTool(t, s, "create_rectangle", map[string]any{})

	callTool(t, s, "create_ellipse", map[string]any{"width": float64(50), "height": float64(50)})
	callTool(t, s, "create_ellipse", map[string]any{})

	callTool(t, s, "create_text", map[string]any{
		"text": "Hello", "fontSize": float64(16), "fontFamily": "Inter", "fontStyle": "Bold",
		"fillColor": "#000000", "name": "Label",
	})

	// import_image with optional params
	callTool(t, s, "import_image", map[string]any{
		"imageData": "abc123", "x": float64(10), "y": float64(20),
		"width": float64(200), "height": float64(150),
		"name": "Hero", "scaleMode": "FILL", "parentId": "1:1",
	})
	// import_image minimal
	callTool(t, s, "import_image", map[string]any{"imageData": "abc123"})
	callTool(t, s, "import_image", map[string]any{"imageUrl": "https://example.com/image.png"})
	callTool(t, s, "create_video", map[string]any{"videoData": "abc123", "scaleMode": "FIT", "exposure": float64(0.25)})
	callTool(t, s, "create_gif", map[string]any{"imageHash": "hash-1", "width": float64(120), "height": float64(80)})
	callTool(t, s, "create_link_preview", map[string]any{"url": "https://example.com", "name": "Docs"})
	callTool(t, s, "create_vector", map[string]any{"vectorPaths": []any{map[string]any{"data": "M 0 0 L 10 10", "windingRule": "NONZERO"}}})
	callTool(t, s, "create_slice", map[string]any{"width": float64(320), "height": float64(180)})
	callTool(t, s, "create_page_divider", map[string]any{"name": "---"})
	callTool(t, s, "create_text_path", map[string]any{"nodeId": "1:1", "startSegment": float64(0), "startPosition": float64(0)})
}

func TestImportImagePrefersBase64InputOverImageURL(t *testing.T) {
	rec := newRecordedRPCServer(t)
	s, _ := newTestServerWithRPCURL(t, rec.server.URL)

	callTool(t, s, "import_image", map[string]any{
		"imageData": "abc123",
		"imageUrl":  "https://example.com/remote.png",
	})

	req := rec.lastRequest(t)
	if _, present := req.Params["imageUrl"]; present {
		t.Fatalf("imageUrl must not be forwarded when imageData is present, got %v", req.Params["imageUrl"])
	}
	if got, _ := req.Params["imageData"].(string); got != "abc123" {
		t.Fatalf("imageData = %q, want abc123", got)
	}
}

// ── Write – modify tools ──────────────────────────────────────────────────────

func TestHandlers_WriteModifyTools(t *testing.T) {
	s, _ := newTestServer(t)

	callTool(t, s, "set_text", map[string]any{"nodeId": "1:1", "text": "Updated"})

	callTool(t, s, "set_fills", map[string]any{
		"nodeId": "1:1", "color": "#FF0000", "opacity": float64(0.8), "mode": "replace",
	})
	callTool(t, s, "set_fills", map[string]any{"nodeId": "1:1", "color": "#00FF00"}) // minimal
	// set_fills — bound to a design variable
	callTool(t, s, "set_fills", map[string]any{"nodeId": "1:1", "color": "#00FF00", "variableId": "VariableID:1:2"})

	callTool(t, s, "set_strokes", map[string]any{
		"nodeId": "1:1", "color": "#000000", "strokeWeight": float64(2), "mode": "append",
	})
	callTool(t, s, "set_strokes", map[string]any{"nodeId": "1:1", "color": "#000000"}) // minimal
	// set_strokes — bound to a design variable
	callTool(t, s, "set_strokes", map[string]any{"nodeId": "1:1", "color": "#000000", "variableId": "VariableID:1:2"})

	callTool(t, s, "move_nodes", map[string]any{"nodeIds": []any{"1:1"}, "x": float64(10), "y": float64(20)})
	callTool(t, s, "move_nodes", map[string]any{"nodeIds": []any{"1:1"}, "x": float64(5)}) // y omitted

	callTool(t, s, "resize_nodes", map[string]any{"nodeIds": []any{"1:1"}, "width": float64(300), "height": float64(200)})
	callTool(t, s, "resize_nodes", map[string]any{"nodeIds": []any{"1:1"}, "height": float64(100)}) // width omitted

	callTool(t, s, "rename_node", map[string]any{"nodeId": "1:1", "name": "New Name"})

	callTool(t, s, "clone_node", map[string]any{"nodeId": "1:1", "x": float64(50), "y": float64(50), "parentId": "2:2"})
	callTool(t, s, "clone_node", map[string]any{"nodeId": "1:1"}) // minimal

	callTool(t, s, "set_file_thumbnail", map[string]any{"nodeId": "1:1"})
	callTool(t, s, "set_file_thumbnail", map[string]any{}) // clear
	callTool(t, s, "add_dev_resource", map[string]any{"nodeId": "1:1", "url": "https://example.com/spec", "name": "Spec"})
	callTool(t, s, "edit_dev_resource", map[string]any{"nodeId": "1:1", "currentUrl": "https://example.com/spec", "url": "https://example.com/new-spec"})
	callTool(t, s, "delete_dev_resource", map[string]any{"nodeId": "1:1", "url": "https://example.com/new-spec"})

	callTool(t, s, "set_auto_layout", map[string]any{"nodeId": "1:1", "layoutMode": "HORIZONTAL"})

	callTool(t, s, "delete_nodes", map[string]any{"nodeIds": []any{"1:1", "2:2"}})
}

// ── Write – style tools ───────────────────────────────────────────────────────

func TestHandlers_WriteStyleTools(t *testing.T) {
	s, _ := newTestServer(t)

	callTool(t, s, "create_paint_style", map[string]any{"name": "Brand/Primary", "color": "#FF5733", "description": "Main brand color"})
	callTool(t, s, "create_text_style", map[string]any{"name": "Heading/H1"})
	callTool(t, s, "create_effect_style", map[string]any{"name": "Elevation/1", "type": "DROP_SHADOW"})
	callTool(t, s, "create_grid_style", map[string]any{"name": "Layout/12col", "pattern": "COLUMNS", "alignment": "STRETCH"})

	callTool(t, s, "update_paint_style", map[string]any{"styleId": "S:abc", "color": "#00FF00"})
	callTool(t, s, "update_paint_style", map[string]any{"styleId": "S:abc", "name": "Renamed"})
	callTool(t, s, "reorder_local_style", map[string]any{"styleType": "PAINT", "styleId": "S:abc", "afterStyleId": "S:def"})
	callTool(t, s, "reorder_local_style_folder", map[string]any{"styleType": "TEXT", "folder": "Heading", "afterFolder": "Body"})
	callTool(t, s, "bind_variable_to_effect", map[string]any{
		"effect": map[string]any{"type": "DROP_SHADOW", "radius": float64(8)}, "field": "radius", "variableId": "v1",
	})
	callTool(t, s, "bind_variable_to_layout_grid", map[string]any{
		"layoutGrid": map[string]any{"pattern": "GRID", "sectionSize": float64(8)}, "field": "sectionSize", "variableId": "v1",
	})

	callTool(t, s, "delete_style", map[string]any{"styleId": "S:abc"})
}

// ── Write – variable tools ────────────────────────────────────────────────────

func TestHandlers_WriteVariableTools(t *testing.T) {
	s, _ := newTestServer(t)

	callTool(t, s, "create_variable_collection", map[string]any{"name": "Brand", "initialModeName": "Light"})
	callTool(t, s, "add_variable_mode", map[string]any{"collectionId": "c1", "modeName": "Dark"})
	callTool(t, s, "create_variable", map[string]any{"name": "primary", "collectionId": "c1", "type": "COLOR"})
	callTool(t, s, "create_variable_alias", map[string]any{"variableId": "v1"})
	callTool(t, s, "set_variable_value", map[string]any{"variableId": "v1", "modeId": "m1", "value": "#fff"})
	callTool(t, s, "update_variable", map[string]any{"variableId": "v1", "removeCodeSyntax": []any{"WEB"}})
	callTool(t, s, "delete_variable", map[string]any{"variableId": "v1"})
	callTool(t, s, "delete_variable", map[string]any{"collectionId": "c1"})
}

// ── Write – component tools ───────────────────────────────────────────────────

func TestHandlers_WriteComponentTools(t *testing.T) {
	s, _ := newTestServer(t)

	callTool(t, s, "swap_component", map[string]any{"nodeId": "1:1", "componentId": "2:2"})
	callTool(t, s, "detach_instance", map[string]any{"nodeIds": []any{"1:1", "2:2"}})
}

// ── Write – linked tools (apply_style_to_node, bind_variable_to_node) ─────────

func TestHandlers_LinkedTools(t *testing.T) {
	s, _ := newTestServer(t)

	callTool(t, s, "apply_style_to_node", map[string]any{"nodeId": "1:1", "styleId": "S:abc", "target": "fill"})
	callTool(t, s, "apply_style_to_node", map[string]any{"nodeId": "1:1", "styleId": "S:abc"}) // no target

	callTool(t, s, "bind_variable_to_node", map[string]any{"nodeId": "1:1", "variableId": "v1", "field": "fills"})
}

func TestHandlers_NodeControlTools(t *testing.T) {
	s, _ := newTestServer(t)

	// set_visible — show
	callTool(t, s, "set_visible", map[string]any{"nodeIds": []any{"1:1"}, "visible": true})
	// set_visible — hide
	callTool(t, s, "set_visible", map[string]any{"nodeIds": []any{"1:1", "2:2"}, "visible": false})

	// lock_nodes
	callTool(t, s, "lock_nodes", map[string]any{"nodeIds": []any{"1:1"}})
	callTool(t, s, "lock_nodes", map[string]any{"nodeIds": []any{"1:1", "2:2"}})

	// unlock_nodes
	callTool(t, s, "unlock_nodes", map[string]any{"nodeIds": []any{"1:1"}})

	// rotate_nodes
	callTool(t, s, "rotate_nodes", map[string]any{"nodeIds": []any{"1:1"}, "rotation": float64(45)})
	callTool(t, s, "rotate_nodes", map[string]any{"nodeIds": []any{"1:1"}, "rotation": float64(-90)})

	// reorder_nodes
	callTool(t, s, "reorder_nodes", map[string]any{"nodeIds": []any{"1:1"}, "order": "bringToFront"})
	callTool(t, s, "reorder_nodes", map[string]any{"nodeIds": []any{"1:1"}, "order": "sendToBack"})
	callTool(t, s, "reorder_nodes", map[string]any{"nodeIds": []any{"1:1"}, "order": "bringForward"})
	callTool(t, s, "reorder_nodes", map[string]any{"nodeIds": []any{"1:1"}, "order": "sendBackward"})

	// set_blend_mode
	callTool(t, s, "set_blend_mode", map[string]any{"nodeIds": []any{"1:1"}, "blendMode": "MULTIPLY"})
	callTool(t, s, "set_blend_mode", map[string]any{"nodeIds": []any{"1:1", "2:2"}, "blendMode": "SCREEN"})

	// set_constraints
	callTool(t, s, "set_constraints", map[string]any{"nodeIds": []any{"1:1"}, "horizontal": "STRETCH"})
	callTool(t, s, "set_constraints", map[string]any{"nodeIds": []any{"1:1"}, "vertical": "CENTER"})
	callTool(t, s, "set_constraints", map[string]any{"nodeIds": []any{"1:1"}, "horizontal": "MIN", "vertical": "MAX"})
}

// ── Write – page management tools ───────────────────────────────────

func TestHandlers_PageManagementTools(t *testing.T) {
	s, _ := newTestServer(t)

	// add_page
	callTool(t, s, "add_page", map[string]any{"name": "Flows"})
	callTool(t, s, "add_page", map[string]any{}) // minimal
	callTool(t, s, "add_page", map[string]any{"name": "Sprint 1", "index": float64(0)})

	// delete_page
	callTool(t, s, "delete_page", map[string]any{"pageId": "0:2"})
	callTool(t, s, "delete_page", map[string]any{"pageName": "Flows"})

	// rename_page
	callTool(t, s, "rename_page", map[string]any{"pageId": "0:2", "newName": "Sprint 1"})
	callTool(t, s, "rename_page", map[string]any{"pageName": "Flows", "newName": "User Flows"})
}

func TestHandlers_ReparentBatchRenameTextReplaceEffectsSection(t *testing.T) {
	s, _ := newTestServer(t)

	// reparent_nodes
	callTool(t, s, "reparent_nodes", map[string]any{"nodeIds": []any{"1:1"}, "parentId": "2:2"})
	callTool(t, s, "reparent_nodes", map[string]any{"nodeIds": []any{"1:1", "3:3"}, "parentId": "2:2"})

	// batch_rename_nodes — find/replace
	callTool(t, s, "batch_rename_nodes", map[string]any{
		"nodeIds": []any{"1:1", "2:2"}, "find": "Button", "replace": "Btn",
	})
	// batch_rename_nodes — prefix/suffix
	callTool(t, s, "batch_rename_nodes", map[string]any{
		"nodeIds": []any{"1:1"}, "prefix": "UI/", "suffix": "_v2",
	})
	// batch_rename_nodes — regex
	callTool(t, s, "batch_rename_nodes", map[string]any{
		"nodeIds": []any{"1:1"}, "find": "\\d+", "replace": "N", "useRegex": true,
	})

	// find_replace_text — across page
	callTool(t, s, "find_replace_text", map[string]any{"find": "Old", "replace": "New"})
	// find_replace_text — scoped to node
	callTool(t, s, "find_replace_text", map[string]any{"find": "x", "replace": "y", "nodeId": "1:1"})
	// find_replace_text — regex
	callTool(t, s, "find_replace_text", map[string]any{
		"find": "\\$\\d+", "replace": "$0", "useRegex": true,
	})

	// set_effects — drop shadow
	callTool(t, s, "set_effects", map[string]any{
		"nodeId":  "1:1",
		"effects": []any{map[string]any{"type": "DROP_SHADOW", "radius": float64(8), "color": "#000000", "opacity": float64(0.3)}},
	})
	// set_effects — layer blur
	callTool(t, s, "set_effects", map[string]any{
		"nodeId":  "1:1",
		"effects": []any{map[string]any{"type": "LAYER_BLUR", "radius": float64(4)}},
	})
	// set_effects — clear
	callTool(t, s, "set_effects", map[string]any{"nodeId": "1:1", "effects": []any{}})

	// create_section
	callTool(t, s, "create_section", map[string]any{"name": "Sprint 1", "x": float64(0), "y": float64(0)})
	callTool(t, s, "create_section", map[string]any{}) // minimal
	callTool(t, s, "create_section", map[string]any{"width": float64(1200), "height": float64(900)})
}

// ── Write – library tools (Track A) ───────────────────────────────────────────

func TestHandlers_LibraryTools(t *testing.T) {
	s, _ := newTestServer(t)

	// import_component_by_key — with optional assetType
	callTool(t, s, "import_component_by_key", map[string]any{"key": strings.Repeat("a", 40), "assetType": "COMPONENT_SET"})
	callTool(t, s, "import_component_by_key", map[string]any{"key": strings.Repeat("b", 40)}) // minimal

	// import_variable_by_key
	callTool(t, s, "import_variable_by_key", map[string]any{"key": "v-key-1"})

	// import_style_by_key
	callTool(t, s, "import_style_by_key", map[string]any{"key": strings.Repeat("c", 40)})

	// create_instance — full
	callTool(t, s, "create_instance", map[string]any{
		"componentId":            "2:2",
		"componentKey":           "ck-1",
		"parentId":               "1:1",
		"index":                  float64(0),
		"x":                      float64(10),
		"y":                      float64(20),
		"width":                  float64(200),
		"height":                 float64(48),
		"layoutSizingHorizontal": "FILL",
		"layoutSizingVertical":   "HUG",
		"variantProperties":      map[string]any{"State": "Default"},
		"properties":             map[string]any{"Label#1:0": "Submit"},
	})
	// create_instance — minimal
	callTool(t, s, "create_instance", map[string]any{"componentId": "2:2"})

	// set_instance_properties
	callTool(t, s, "set_instance_properties", map[string]any{
		"nodeId":     "1:1",
		"properties": map[string]any{"State": "On"},
	})
	callTool(t, s, "set_instance_properties", map[string]any{
		"nodeId":         "1:1",
		"properties":     map[string]any{"State": "On"},
		"resetOverrides": true,
	})

	// set_variable_mode
	callTool(t, s, "set_variable_mode", map[string]any{
		"nodeId": "1:1", "collectionId": "c1", "modeId": "m1",
	})

	// get_remote_variable_collection
	callTool(t, s, "get_remote_variable_collection", map[string]any{"collectionId": "c1"})

	// list_library_variable_collections — no params
	callTool(t, s, "list_library_variable_collections", nil)
}

func TestHandlers_ImportComponentByKeyRejectsBadKeysBeforeRPC(t *testing.T) {
	resetLibraryCatalogIndexForTest()
	rec := newRecordedRPCServer(t)
	s, _ := newTestServerWithRPCURL(t, rec.server.URL)

	tests := []struct {
		name    string
		key     string
		wantMsg string
	}{
		{name: "truncated", key: "8b931898634bdc63", wantMsg: "truncated"},
		{name: "node id", key: "410:49695", wantMsg: "node id"},
		{name: "non hex", key: strings.Repeat("z", 40), wantMsg: "malformed component key"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := callToolResult(t, s, "import_component_by_key", map[string]any{"key": tt.key})
			text := resultText(t, result)
			if !result.IsError {
				t.Fatalf("expected tool error for %s", tt.name)
			}
			if !strings.Contains(text, tt.wantMsg) {
				t.Fatalf("error text %q does not contain %q", text, tt.wantMsg)
			}
			if got := rec.count(); got != 0 {
				t.Fatalf("invalid key reached RPC %d time(s)", got)
			}
		})
	}
}

func TestHandlers_ImportComponentByKeyValidKeyReachesRPC(t *testing.T) {
	resetLibraryCatalogIndexForTest()
	rec := newRecordedRPCServer(t)
	s, _ := newTestServerWithRPCURL(t, rec.server.URL)
	key := strings.Repeat("a", 40)

	result := callToolResult(t, s, "import_component_by_key", map[string]any{"key": key})
	if result.IsError {
		t.Fatalf("valid key unexpectedly failed: %s", resultText(t, result))
	}
	if got := rec.count(); got != 1 {
		t.Fatalf("valid key RPC count = %d, want 1", got)
	}
	req := rec.lastRequest(t)
	if req.Tool != "import_component_by_key" {
		t.Fatalf("tool = %q, want import_component_by_key", req.Tool)
	}
	if req.Params["key"] != key {
		t.Fatalf("forwarded key = %v, want %s", req.Params["key"], key)
	}
	if _, ok := req.Params["assetType"]; ok {
		t.Fatalf("assetType should not be injected without a cached catalog, got %v", req.Params["assetType"])
	}
}

func TestHandlers_ImportComponentByKeyInjectsCatalogAssetType(t *testing.T) {
	resetLibraryCatalogIndexForTest()
	rec := newRecordedRPCServer(t)
	s, _ := newTestServerWithRPCURL(t, rec.server.URL)
	key := strings.Repeat("d", 40)
	rememberLibraryCatalogKeys(map[string]any{
		key: map[string]any{"type": "COMPONENT_SET", "name": "Button"},
	})

	result := callToolResult(t, s, "import_component_by_key", map[string]any{"key": key})
	if result.IsError {
		t.Fatalf("valid component set key unexpectedly failed: %s", resultText(t, result))
	}
	req := rec.lastRequest(t)
	if req.Params["assetType"] != "COMPONENT_SET" {
		t.Fatalf("assetType = %v, want COMPONENT_SET", req.Params["assetType"])
	}
}

func TestHandlers_ImportVariableByKeyRejectsNodeIDBeforeRPC(t *testing.T) {
	resetLibraryCatalogIndexForTest()
	rec := newRecordedRPCServer(t)
	s, _ := newTestServerWithRPCURL(t, rec.server.URL)

	result := callToolResult(t, s, "import_variable_by_key", map[string]any{"key": "410:49695"})
	text := resultText(t, result)
	if !result.IsError {
		t.Fatal("expected node-id variable key to fail")
	}
	if !strings.Contains(text, "node id") {
		t.Fatalf("error text should mention node id, got %q", text)
	}
	if got := rec.count(); got != 0 {
		t.Fatalf("invalid variable key reached RPC %d time(s)", got)
	}
}

func TestHandlers_ImportStyleByKeyRejectsBadKeyBeforeRPC(t *testing.T) {
	resetLibraryCatalogIndexForTest()
	rec := newRecordedRPCServer(t)
	s, _ := newTestServerWithRPCURL(t, rec.server.URL)

	result := callToolResult(t, s, "import_style_by_key", map[string]any{"key": "410:49695"})
	if !result.IsError {
		t.Fatal("expected node-id style key to fail")
	}
	if text := resultText(t, result); !strings.Contains(text, "node id") {
		t.Fatalf("error text should mention node id, got %q", text)
	}
	if got := rec.count(); got != 0 {
		t.Fatalf("invalid style key reached RPC %d time(s)", got)
	}
}

func TestHandlers_ImportStyleAndVariableValidKeysReachRPC(t *testing.T) {
	resetLibraryCatalogIndexForTest()
	rec := newRecordedRPCServer(t)
	s, _ := newTestServerWithRPCURL(t, rec.server.URL)

	styleKey := strings.Repeat("e", 40)
	styleResult := callToolResult(t, s, "import_style_by_key", map[string]any{"key": styleKey})
	if styleResult.IsError {
		t.Fatalf("valid style key unexpectedly failed: %s", resultText(t, styleResult))
	}
	variableResult := callToolResult(t, s, "import_variable_by_key", map[string]any{"key": "VariableID:123:456"})
	if variableResult.IsError {
		t.Fatalf("valid variable key unexpectedly failed: %s", resultText(t, variableResult))
	}
	if got := rec.count(); got != 2 {
		t.Fatalf("valid style + variable RPC count = %d, want 2", got)
	}
}
