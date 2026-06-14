package internal

import (
	"context"
	"fmt"
	"log"
	"os"
	"sync"
)

var nodeLogger = log.New(os.Stderr, "[node] ", 0)

// Node dynamically routes MCP tool calls to either the Leader bridge
// or the Follower HTTP proxy, depending on the current role.
type Node struct {
	mu       sync.RWMutex
	role     Role
	ip       string
	port     int
	leader   *Leader
	follower *Follower
	version  string
}

// NewNode creates a Node in the Unknown role.
func NewNode(ip string, port int, version string) *Node {
	return &Node{
		ip:       ip,
		port:     port,
		role:     RoleUnknown,
		version:  version,
		follower: NewFollower(fmt.Sprintf("http://%s:%d", ip, port)),
	}
}

// Role returns the current role.
func (n *Node) Role() Role {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.role
}

// RoleName returns a human-readable role string.
func (n *Node) RoleName() string {
	switch n.Role() {
	case RoleLeader:
		return "LEADER"
	case RoleFollower:
		return "FOLLOWER"
	default:
		return "UNKNOWN"
	}
}

// Send routes a request to the appropriate backend.
func (n *Node) Send(ctx context.Context, tool string, nodeIDs []string, params map[string]interface{}) (BridgeResponse, error) {
	n.mu.RLock()
	role := n.role
	leader := n.leader
	n.mu.RUnlock()

	// Normalize hyphen-format node IDs that LLMs sometimes produce.
	for i, id := range nodeIDs {
		nodeIDs[i] = NormalizeNodeID(id)
	}
	// Normalize common param keys that contain node IDs.
	for _, key := range []string{"nodeId", "parentId"} {
		if v, ok := params[key].(string); ok {
			params[key] = NormalizeNodeID(v)
		}
	}

	nodeLogger.Printf("tool=%s role=%s nodeIDs=%v", tool, n.RoleName(), nodeIDs)

	var resp BridgeResponse
	var err error
	if role == RoleLeader && leader != nil {
		resp, err = leader.GetBridge().Send(ctx, tool, nodeIDs, params)
	} else {
		resp, err = n.follower.Send(ctx, tool, nodeIDs, params)
	}

	// Attach a self-correction hint to failures so the LLM can recover without
	// a human (retry smaller / use REST / re-confirm a stale id). This is the
	// single chokepoint every tool handler funnels through and it knows the
	// request type. The follower→leader RPC path resolves through bridge.Send
	// directly (leader.handleRPC), so a hint is added exactly once here.
	msg := resp.Error
	if err != nil {
		msg = err.Error()
	}
	if h := hintFor(tool, msg); h != "" {
		if err != nil {
			err = fmt.Errorf("%w\n\n💡 %s", err, h)
		} else {
			resp.Error += "\n\n💡 " + h
		}
	}
	return resp, err
}

// ListChannels returns the connected plugin channels (one per open Figma file).
// On the leader it reads the bridge directly; on a follower it asks the leader.
func (n *Node) ListChannels(ctx context.Context) ([]ChannelInfo, error) {
	n.mu.RLock()
	role := n.role
	leader := n.leader
	n.mu.RUnlock()

	if role == RoleLeader && leader != nil {
		return leader.GetBridge().ListChannels(), nil
	}
	return n.follower.ListChannels(ctx)
}

// BecomeLeader attempts to bind the port and transition to Leader role.
// Returns an error if the port is already in use.
func (n *Node) BecomeLeader() error {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.role == RoleLeader {
		return nil
	}

	leader := NewLeader(n.ip, n.port, n.version)
	if err := leader.Start(); err != nil {
		return err
	}

	n.leader = leader
	n.role = RoleLeader
	nodeLogger.Printf("became LEADER")
	return nil
}

// BecomeFollower transitions to Follower role, stopping the leader if running.
func (n *Node) BecomeFollower() {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.role == RoleFollower {
		return
	}

	if n.leader != nil {
		n.leader.Stop()
		n.leader = nil
	}

	n.role = RoleFollower
	nodeLogger.Printf("became FOLLOWER")
}

// Stop shuts down the node regardless of role.
func (n *Node) Stop() {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.leader != nil {
		n.leader.Stop()
		n.leader = nil
	}
	n.role = RoleUnknown
}
