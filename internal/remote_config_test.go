package internal

import (
	"strings"
	"testing"
)

func TestNewRemoteFollowerConfig_accepts_valid_inputs(t *testing.T) {
	// Given
	leaderURL := "https://design-mac.example.ts.net"
	proxyURL := "http://127.0.0.1:45123"
	mcpListen := "127.0.0.1:45124"

	// When
	cfg, err := NewRemoteFollowerConfig(leaderURL, proxyURL, mcpListen)

	// Then
	if err != nil {
		t.Fatalf("NewRemoteFollowerConfig: %v", err)
	}
	if cfg.LeaderURL != leaderURL {
		t.Fatalf("LeaderURL = %q, want %q", cfg.LeaderURL, leaderURL)
	}
	if cfg.OutboundProxy != proxyURL {
		t.Fatalf("OutboundProxy = %q, want %q", cfg.OutboundProxy, proxyURL)
	}
	if cfg.MCPListen != mcpListen {
		t.Fatalf("MCPListen = %q, want %q", cfg.MCPListen, mcpListen)
	}
}

func TestNewRemoteFollowerConfig_rejects_invalid_leader_urls(t *testing.T) {
	tests := []struct {
		name   string
		leader string
	}{
		{"http scheme", "http://design-mac.example.ts.net"},
		{"non tailscale host", "https://example.com"},
		{"userinfo", "https://user@design-mac.example.ts.net"},
		{"query", "https://design-mac.example.ts.net?x=1"},
		{"fragment", "https://design-mac.example.ts.net#frag"},
		{"unexpected path", "https://design-mac.example.ts.net/rpc"},
		{"malformed port", "https://design-mac.example.ts.net:bad"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// When
			_, err := NewRemoteFollowerConfig(tt.leader, "http://127.0.0.1:45123", "127.0.0.1:45124")

			// Then
			if err == nil {
				t.Fatalf("expected invalid leader %q to fail", tt.leader)
			}
		})
	}
}

func TestNewRemoteFollowerConfig_rejects_invalid_proxy_urls(t *testing.T) {
	tests := []struct {
		name  string
		proxy string
	}{
		{"remote host", "http://10.0.0.1:45123"},
		{"localhost name", "http://localhost:45123"},
		{"credentials", "http://user:pass@127.0.0.1:45123"},
		{"https scheme", "https://127.0.0.1:45123"},
		{"path", "http://127.0.0.1:45123/proxy"},
		{"query", "http://127.0.0.1:45123?x=1"},
		{"fragment", "http://127.0.0.1:45123#frag"},
		{"missing port", "http://127.0.0.1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// When
			_, err := NewRemoteFollowerConfig("https://design-mac.example.ts.net", tt.proxy, "127.0.0.1:45124")

			// Then
			if err == nil {
				t.Fatalf("expected invalid proxy %q to fail", tt.proxy)
			}
		})
	}
}

func TestNewRemoteFollowerConfig_rejects_non_loopback_mcp_listen(t *testing.T) {
	tests := []string{
		"0.0.0.0:45124",
		"10.0.0.5:45124",
		"localhost:45124",
		"[::1]:45124",
		"127.0.0.1",
		"127.0.0.1:bad",
	}
	for _, listen := range tests {
		t.Run(strings.ReplaceAll(listen, ":", "_"), func(t *testing.T) {
			// When
			_, err := NewRemoteFollowerConfig("https://design-mac.example.ts.net", "http://127.0.0.1:45123", listen)

			// Then
			if err == nil {
				t.Fatalf("expected invalid listen address %q to fail", listen)
			}
		})
	}
}
