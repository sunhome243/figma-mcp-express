package internal

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"
)

var followerLogger = log.New(os.Stderr, "[follower] ", 0)

// Follower proxies MCP tool calls to the leader via HTTP /rpc.
type Follower struct {
	leaderURL string
	client    *http.Client
}

// followerClientTimeout derives a safe HTTP client timeout for the follower.
// It uses parseRequestTimeout() (FIGMA_MCP_TIMEOUT, default 120s), lifts the
// floor to 120s to cover the get_document special-case allowance, then adds 5s
// headroom so the leader always times out before the follower's HTTP client
// drops the connection.
func followerClientTimeout() time.Duration {
	t := parseRequestTimeout()
	if t < 120*time.Second {
		t = 120 * time.Second // cover get_document's 120s allowance
	}
	return t + 5*time.Second // headroom so leader times out first
}

// NewFollower creates a Follower pointed at the given leader base URL.
func NewFollower(leaderURL string) *Follower {
	return &Follower{
		leaderURL: leaderURL,
		client: &http.Client{
			// Tracks FIGMA_MCP_TIMEOUT (default 120s), floored at 120s to cover
			// get_document's extended allowance, plus 5s headroom so the leader
			// always times out before the follower's HTTP client drops the connection.
			Timeout: followerClientTimeout(),
		},
	}
}

// Send proxies a tool call to the leader.
func (f *Follower) Send(ctx context.Context, tool string, nodeIDs []string, params map[string]interface{}) (BridgeResponse, error) {
	followerLogger.Printf("proxy %s nodeIDs=%v params=%v → %s/rpc", tool, nodeIDs, params, f.leaderURL)
	start := time.Now()

	rpcReq := RPCRequest{
		Tool:    tool,
		NodeIDs: nodeIDs,
		Params:  params,
	}

	body, err := json.Marshal(rpcReq)
	if err != nil {
		return BridgeResponse{}, fmt.Errorf("marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, f.leaderURL+"/rpc", bytes.NewReader(body))
	if err != nil {
		return BridgeResponse{}, fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := f.client.Do(req)
	if err != nil {
		followerLogger.Printf("proxy %s rpc error: %v", tool, err)
		return BridgeResponse{}, fmt.Errorf("rpc call: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return BridgeResponse{}, fmt.Errorf("read response: %w", err)
	}

	var rpcResp RPCResponse
	if err := json.Unmarshal(respBody, &rpcResp); err != nil {
		return BridgeResponse{}, fmt.Errorf("unmarshal: %w", err)
	}

	if rpcResp.Error != "" {
		followerLogger.Printf("proxy %s error from leader in %dms: %s", tool, time.Since(start).Milliseconds(), rpcResp.Error)
		return BridgeResponse{Error: rpcResp.Error}, nil
	}

	followerLogger.Printf("proxy %s ok in %dms", tool, time.Since(start).Milliseconds())
	return BridgeResponse{
		Type: tool,
		Data: rpcResp.Data,
	}, nil
}

// ListChannels fetches the connected plugin channels from the leader.
func (f *Follower) ListChannels(ctx context.Context) ([]ChannelInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, f.leaderURL+"/channels", nil)
	if err != nil {
		return nil, fmt.Errorf("new request: %w", err)
	}
	resp, err := f.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("channels call: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
	var infos []ChannelInfo
	if err := json.Unmarshal(body, &infos); err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
	}
	return infos, nil
}

// Ping checks if the leader is alive. Returns true if healthy.
func (f *Follower) Ping(ctx context.Context) bool {
	healthy, _ := f.PingVersion(ctx)
	return healthy
}

// PingVersion checks if the leader is alive AND reports the leader's build
// version (from the /ping JSON body). Returns (healthy, version). version is ""
// when the body can't be decoded — callers treat an unknown version as "don't
// take over" (follow), so a decode miss degrades to today's behavior. Used by
// the election to evict a stale older leader (issue #26).
func (f *Follower) PingVersion(ctx context.Context) (bool, string) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, f.leaderURL+"/ping", nil)
	if err != nil {
		followerLogger.Printf("ping new request error: %v", err)
		return false, ""
	}

	resp, err := f.client.Do(req)
	if err != nil {
		followerLogger.Printf("ping %s failed: %v", f.leaderURL, err)
		return false, ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		followerLogger.Printf("ping %s → %d (healthy=false)", f.leaderURL, resp.StatusCode)
		return false, ""
	}

	var body struct {
		Status  string `json:"status"`
		Version string `json:"version"`
	}
	// A decode error leaves version "" — still healthy, just version-unknown.
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		followerLogger.Printf("ping %s → 200 but body decode failed: %v", f.leaderURL, err)
	}
	followerLogger.Printf("ping %s → 200 (healthy=true, version=%q)", f.leaderURL, body.Version)
	return true, body.Version
}
