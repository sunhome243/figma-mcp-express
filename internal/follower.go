package internal

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

var followerLogger = log.New(os.Stderr, "[follower] ", 0)

var (
	ErrFollowerResponseTooLarge = errors.New("follower response too large")
	ErrFollowerUpstreamStatus   = errors.New("follower upstream status")
)

const (
	maxPingResponseBytes     = 4 << 10
	maxChannelsResponseBytes = 4 << 20
	maxRPCResponseBytes      = 64 << 20
)

// Follower proxies MCP tool calls to the leader via HTTP /rpc.
type Follower struct {
	leaderURL string
	client    *http.Client
}

type FollowerTransportError struct {
	Operation string
	Err       error
}

func (e *FollowerTransportError) Error() string {
	return e.Operation + " transport failed"
}

func (e *FollowerTransportError) Unwrap() error {
	return e.Err
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
	return NewFollowerWithClient(leaderURL, defaultFollowerHTTPClient())
}

func NewFollowerWithClient(leaderURL string, client *http.Client) *Follower {
	if client == nil {
		client = defaultFollowerHTTPClient()
	}
	return &Follower{
		leaderURL: strings.TrimRight(leaderURL, "/"),
		client:    client,
	}
}

func defaultFollowerHTTPClient() *http.Client {
	return &http.Client{
		// Tracks FIGMA_MCP_TIMEOUT (default 120s), floored at 120s to cover
		// get_document's extended allowance, plus 5s headroom so the leader
		// always times out before the follower's HTTP client drops the connection.
		Timeout: followerClientTimeout(),
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

// Send proxies a tool call to the leader.
func (f *Follower) Send(ctx context.Context, tool string, nodeIDs []string, params map[string]interface{}) (BridgeResponse, error) {
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

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, f.endpoint("/rpc"), bytes.NewReader(body))
	if err != nil {
		return BridgeResponse{}, fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := f.client.Do(req)
	if err != nil {
		followerLogger.Printf("tool=%s status=transport_error duration_ms=%d", tool, time.Since(start).Milliseconds())
		return BridgeResponse{}, &FollowerTransportError{Operation: "rpc", Err: err}
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		followerLogger.Printf("tool=%s status=upstream_status duration_ms=%d", tool, time.Since(start).Milliseconds())
		return BridgeResponse{}, fmt.Errorf("rpc status %d: %w", resp.StatusCode, ErrFollowerUpstreamStatus)
	}

	var rpcResp RPCResponse
	if err := decodeLimitedJSON(resp.Body, maxRPCResponseBytes, &rpcResp); err != nil {
		followerLogger.Printf("tool=%s status=decode_error duration_ms=%d", tool, time.Since(start).Milliseconds())
		return BridgeResponse{}, fmt.Errorf("rpc decode: %w", err)
	}

	if rpcResp.Error != "" {
		followerLogger.Printf("tool=%s status=leader_error duration_ms=%d", tool, time.Since(start).Milliseconds())
		return BridgeResponse{Error: rpcResp.Error}, nil
	}

	followerLogger.Printf("tool=%s status=ok duration_ms=%d", tool, time.Since(start).Milliseconds())
	return BridgeResponse{
		Type: tool,
		Data: rpcResp.Data,
	}, nil
}

// ListChannels fetches the connected plugin channels from the leader.
func (f *Follower) ListChannels(ctx context.Context) ([]ChannelInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, f.endpoint("/channels"), nil)
	if err != nil {
		return nil, fmt.Errorf("new request: %w", err)
	}
	resp, err := f.client.Do(req)
	if err != nil {
		followerLogger.Printf("tool=list_channels status=transport_error")
		return nil, &FollowerTransportError{Operation: "channels", Err: err}
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		followerLogger.Printf("tool=list_channels status=upstream_status")
		return nil, fmt.Errorf("channels status %d: %w", resp.StatusCode, ErrFollowerUpstreamStatus)
	}
	var infos []ChannelInfo
	if err := decodeLimitedJSON(resp.Body, maxChannelsResponseBytes, &infos); err != nil {
		followerLogger.Printf("tool=list_channels status=decode_error")
		return nil, fmt.Errorf("channels decode: %w", err)
	}
	followerLogger.Printf("tool=list_channels status=ok")
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

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, f.endpoint("/ping"), nil)
	if err != nil {
		followerLogger.Printf("tool=ping status=request_error")
		return false, ""
	}

	resp, err := f.client.Do(req)
	if err != nil {
		followerLogger.Printf("tool=ping status=transport_error")
		return false, ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		followerLogger.Printf("tool=ping status=upstream_status")
		return false, ""
	}

	var body struct {
		Status  string `json:"status"`
		Version string `json:"version"`
	}
	// A decode error leaves version "" — still healthy, just version-unknown.
	if err := decodeLimitedJSON(resp.Body, maxPingResponseBytes, &body); err != nil {
		followerLogger.Printf("tool=ping status=decode_error")
	}
	followerLogger.Printf("tool=ping status=ok")
	return true, body.Version
}

func (f *Follower) endpoint(path string) string {
	return f.leaderURL + path
}

func decodeLimitedJSON(r io.Reader, maxBytes int64, dest interface{}) error {
	limited := io.LimitReader(r, maxBytes+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}
	if int64(len(body)) > maxBytes {
		return ErrFollowerResponseTooLarge
	}
	if err := json.Unmarshal(body, dest); err != nil {
		return fmt.Errorf("unmarshal response: %w", err)
	}
	return nil
}
