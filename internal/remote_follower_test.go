package internal

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestFollower_usesDedicatedOutboundProxy(t *testing.T) {
	// Given
	var proxySawRPC bool
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Scheme != "http" || r.URL.Host != "design-mac.example.ts.net" || r.URL.Path != "/rpc" {
			t.Fatalf("proxy saw unexpected request URL %q", r.URL.String())
		}
		proxySawRPC = true
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(RPCResponse{Data: "ok"}) //nolint:errcheck
	}))
	t.Cleanup(proxy.Close)

	proxyURL, err := url.Parse(proxy.URL)
	if err != nil {
		t.Fatalf("parse proxy URL: %v", err)
	}
	client := &http.Client{
		Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)},
	}
	f := NewFollowerWithClient("http://design-mac.example.ts.net", client)

	// When
	resp, err := f.Send(context.Background(), "get_metadata", nil, nil)

	// Then
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if resp.Error != "" {
		t.Fatalf("unexpected bridge error: %s", resp.Error)
	}
	if !proxySawRPC {
		t.Fatal("proxy did not receive /rpc request")
	}
}

func TestFollower_usesDedicatedOutboundProxyForPingAndChannels(t *testing.T) {
	// Given
	seen := map[string]bool{}
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Scheme != "http" || r.URL.Host != "design-mac.example.ts.net" {
			t.Fatalf("proxy saw unexpected request URL %q", r.URL.String())
		}
		seen[r.URL.Path] = true
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/ping":
			json.NewEncoder(w).Encode(map[string]string{"status": "ok", "version": "test"}) //nolint:errcheck
		case "/channels":
			json.NewEncoder(w).Encode([]ChannelInfo{{Channel: "chan-1", FileName: "Remote File"}}) //nolint:errcheck
		default:
			t.Fatalf("proxy saw unexpected path %q", r.URL.Path)
		}
	}))
	t.Cleanup(proxy.Close)

	proxyURL, err := url.Parse(proxy.URL)
	if err != nil {
		t.Fatalf("parse proxy URL: %v", err)
	}
	f := NewFollowerWithClient("http://design-mac.example.ts.net", &http.Client{
		Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)},
	})

	// When
	ok, version := f.PingVersion(context.Background())
	channels, err := f.ListChannels(context.Background())

	// Then
	if !ok || version != "test" {
		t.Fatalf("PingVersion = (%v, %q), want (true, test)", ok, version)
	}
	if err != nil {
		t.Fatalf("ListChannels: %v", err)
	}
	if len(channels) != 1 || channels[0].Channel != "chan-1" {
		t.Fatalf("channels = %#v, want chan-1", channels)
	}
	for _, path := range []string{"/ping", "/channels"} {
		if !seen[path] {
			t.Fatalf("proxy did not receive %s", path)
		}
	}
}

func TestFollowerProxy_doesNotAffectUnrelatedHTTPClients(t *testing.T) {
	// Given
	proxy := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("unrelated default HTTP client must not use follower proxy")
	}))
	t.Cleanup(proxy.Close)

	cfg, err := NewRemoteFollowerConfig("https://design-mac.example.ts.net", proxy.URL, "127.0.0.1:45124")
	if err != nil {
		t.Fatalf("NewRemoteFollowerConfig: %v", err)
	}
	if _, err := NewRemoteFollowerHTTPClient(cfg); err != nil {
		t.Fatalf("NewRemoteFollowerHTTPClient: %v", err)
	}

	direct := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(direct.Close)

	// When
	resp, err := http.Get(direct.URL) //nolint:gosec,noctx

	// Then
	if err != nil {
		t.Fatalf("http.Get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusNoContent)
	}
}

func TestFollowerListChannels_decodesGzipResponse(t *testing.T) {
	// Given
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Set("Content-Type", "application/json")
		gz := gzip.NewWriter(w)
		defer gz.Close()
		json.NewEncoder(gz).Encode([]ChannelInfo{{Channel: "chan-1", FileName: "Remote File"}}) //nolint:errcheck
	}))
	t.Cleanup(srv.Close)

	f := NewFollower(srv.URL)

	// When
	channels, err := f.ListChannels(context.Background())

	// Then
	if err != nil {
		t.Fatalf("ListChannels: %v", err)
	}
	if len(channels) != 1 || channels[0].Channel != "chan-1" {
		t.Fatalf("channels = %#v, want chan-1", channels)
	}
}

func TestFollowerSend_rejectsNon2xxBeforeDecode(t *testing.T) {
	// Given
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "raw upstream body must stay private", http.StatusBadGateway)
	}))
	t.Cleanup(srv.Close)

	f := NewFollower(srv.URL)

	// When
	_, err := f.Send(context.Background(), "get_metadata", nil, nil)

	// Then
	if err == nil {
		t.Fatal("expected non-2xx response to fail")
	}
	if containsCI(err.Error(), "raw upstream body") {
		t.Fatalf("error leaked upstream body: %v", err)
	}
}

func TestFollowerSend_timeoutErrorDoesNotLeakSensitiveRequestData(t *testing.T) {
	// Given
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		time.Sleep(100 * time.Millisecond)
	}))
	t.Cleanup(srv.Close)

	f := NewFollowerWithClient(srv.URL+"/secret-peer", &http.Client{Timeout: time.Millisecond})

	// When
	_, err := f.Send(context.Background(), "set_text", []string{"1:1"}, map[string]any{
		"text":     "secret body",
		"fileKey":  "secret-file",
		"origin":   "grace",
		"channel":  "secret-channel",
		"nodeName": "secret-node",
	})

	// Then
	if err == nil {
		t.Fatal("expected timeout to fail")
	}
	for _, forbidden := range []string{
		srv.URL,
		"secret-peer",
		"1:1",
		"secret body",
		"secret-file",
		"secret-channel",
		"secret-node",
	} {
		if strings.Contains(err.Error(), forbidden) {
			t.Fatalf("timeout error leaked %q in %q", forbidden, err.Error())
		}
	}
}

func TestFollowerSend_decodesGzipRPCResponse(t *testing.T) {
	// Given
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Set("Content-Type", "application/json")
		gz := gzip.NewWriter(w)
		defer gz.Close()
		json.NewEncoder(gz).Encode(RPCResponse{Data: map[string]any{"ok": true}}) //nolint:errcheck
	}))
	t.Cleanup(srv.Close)

	f := NewFollower(srv.URL)

	// When
	resp, err := f.Send(context.Background(), "get_metadata", nil, nil)

	// Then
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if resp.Error != "" {
		t.Fatalf("unexpected bridge error: %s", resp.Error)
	}
}

func TestDecodeLimitedJSON_rejectsOversizedResponse(t *testing.T) {
	// Given
	body := strings.NewReader(`{"data":"` + strings.Repeat("x", 32) + `"}`)

	// When
	err := decodeLimitedJSON(body, 8, &RPCResponse{})

	// Then
	if err == nil {
		t.Fatal("expected oversized response to fail")
	}
	if err != ErrFollowerResponseTooLarge {
		t.Fatalf("error = %v, want %v", err, ErrFollowerResponseTooLarge)
	}
}

func TestFollowerSend_responseLossAfterMutationDoesNotReplay(t *testing.T) {
	// Given
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if _, err := io.ReadAll(r.Body); err != nil {
			t.Errorf("read body: %v", err)
		}
		hj, ok := w.(http.Hijacker)
		if !ok {
			t.Fatal("response writer does not support hijacking")
		}
		conn, _, err := hj.Hijack()
		if err != nil {
			t.Fatalf("hijack: %v", err)
		}
		conn.Close() //nolint:errcheck
	}))
	t.Cleanup(srv.Close)

	f := NewFollower(srv.URL)

	// When
	_, err := f.Send(context.Background(), "set_text", []string{"1:1"}, map[string]any{"text": "updated"})

	// Then
	if err == nil {
		t.Fatal("expected response loss to fail")
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("rpc requests = %d, want exactly 1", got)
	}
}

func TestFollowerLogs_doNotLeakSensitiveRequestOrResponseData(t *testing.T) {
	// Given
	var logs bytes.Buffer
	oldLogger := followerLogger
	followerLogger = log.New(&logs, "", 0)
	t.Cleanup(func() { followerLogger = oldLogger })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(RPCResponse{Error: "raw upstream payload"}) //nolint:errcheck
	}))
	t.Cleanup(srv.Close)

	f := NewFollower(srv.URL)

	// When
	_, err := f.Send(context.Background(), "get_node", []string{"1:1"}, map[string]any{
		"origin":  "grace",
		"channel": "secret-channel",
	})

	// Then
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	got := logs.String()
	for _, forbidden := range []string{
		srv.URL,
		"1:1",
		"grace",
		"secret-channel",
		"raw upstream payload",
		"params",
		"nodeIDs",
	} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("log leaked %q in %q", forbidden, got)
		}
	}
	if !strings.Contains(got, "tool=get_node status=leader_error") {
		t.Fatalf("log = %q, want stable status shape", got)
	}
}
