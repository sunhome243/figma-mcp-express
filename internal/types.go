package internal

// BridgeRequest is sent from the Go server to the Figma plugin over WebSocket.
type BridgeRequest struct {
	Type      string                 `json:"type"`
	RequestID string                 `json:"requestId"`
	NodeIDs   []string               `json:"nodeIds,omitempty"`
	Params    map[string]interface{} `json:"params,omitempty"`
}

// BridgeResponse is received from the Figma plugin over WebSocket.
type BridgeResponse struct {
	Type      string      `json:"type"`
	RequestID string      `json:"requestId"`
	Data      interface{} `json:"data,omitempty"`
	Error     string      `json:"error,omitempty"`
	// Progress fields — sent mid-operation for long-running commands
	Progress int    `json:"progress,omitempty"`
	Message  string `json:"message,omitempty"`

	// Queue-visibility metadata (C1). Populated by the bridge, NOT the plugin, so a
	// queued request (and the LLM) can tell it waited rather than hung. queueWaitMs is
	// how long this request sat in the per-channel serial slot before acquiring it;
	// queueDepth is the number of OTHER requests already waiting on that slot at the
	// moment this one acquired it. Both are zero for an uncontended request and for a
	// served-from-cache read (which never queues). Surfaced into resp.Data by
	// renderResponse only when Data is already a map (non-breaking).
	QueueWaitMs int64 `json:"queueWaitMs,omitempty"`
	QueueDepth  int   `json:"queueDepth,omitempty"`
}

// presenceQueueType is the BridgeRequest-shaped frame Type the bridge pushes to
// the plugin to report which roster origins are currently waiting on the per-
// channel serial slot (the multi-agent live-presence "queued" signal).
const presenceQueueType = "presence_queue"

// PresenceQueueFrame is pushed from the Go server to the Figma plugin to surface
// which roster origins are currently QUEUED — waiting on a channel's serial slot
// (connEntry.sem). HARD CONTRACT: the plugin matches these field names/casing
// exactly. Origins is always a non-nil slice (empty when nobody waits) so the
// plugin can clear its queued list on the empty frame.
type PresenceQueueFrame struct {
	Type    string   `json:"type"`    // always "presence_queue"
	Channel string   `json:"channel"`
	Origins []string `json:"origins"` // distinct roster origins currently waiting on sem
}

// RPCRequest is the wire format for follower → leader /rpc calls.
type RPCRequest struct {
	Tool    string                 `json:"tool"`
	NodeIDs []string               `json:"nodeIds,omitempty"`
	Params  map[string]interface{} `json:"params,omitempty"`
}

// RPCResponse is returned by the leader /rpc endpoint.
type RPCResponse struct {
	Data  interface{} `json:"data,omitempty"`
	Error string      `json:"error,omitempty"`
}

// ChannelInfo describes one connected Figma plugin (one per open file).
type ChannelInfo struct {
	Channel     string `json:"channel"`
	FileName    string `json:"fileName"`
	FileKey     string `json:"fileKey,omitempty"`
	PageName    string `json:"pageName,omitempty"`
	ConnectedAt int64  `json:"connectedAt"`
}

// registerMessageType is the BridgeResponse.Type a plugin sends right after
// connecting to attach file metadata (fileName/fileKey/pageName) to its channel.
const registerMessageType = "__register__"

// progressUpdateType is the BridgeResponse.Type the plugin sends mid-operation
// to indicate progress and reset the Go-bridge per-request timeout.
// A progress_update message must NEVER resolve or delete a pending request.
const progressUpdateType = "progress_update"

// Role represents the current role of this server process.
type Role int

const (
	RoleUnknown  Role = 0
	RoleLeader   Role = 1
	RoleFollower Role = 2
)
