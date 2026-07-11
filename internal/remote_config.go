package internal

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

var ErrInvalidRemoteConfig = errors.New("invalid remote follower config")

type RemoteFollowerConfig struct {
	LeaderURL     string
	OutboundProxy string
	MCPListen     string
}

func NewRemoteFollowerConfig(leaderURL, outboundProxy, mcpListen string) (RemoteFollowerConfig, error) {
	leader, err := parseRemoteLeaderURL(leaderURL)
	if err != nil {
		return RemoteFollowerConfig{}, err
	}
	proxy, err := parseRemoteProxyURL(outboundProxy)
	if err != nil {
		return RemoteFollowerConfig{}, err
	}
	listen, err := parseRemoteMCPListen(mcpListen)
	if err != nil {
		return RemoteFollowerConfig{}, err
	}
	return RemoteFollowerConfig{
		LeaderURL:     leader,
		OutboundProxy: proxy,
		MCPListen:     listen,
	}, nil
}

func NewRemoteFollowerHTTPClient(cfg RemoteFollowerConfig) (*http.Client, error) {
	proxyURL, err := url.Parse(cfg.OutboundProxy)
	if err != nil {
		return nil, fmt.Errorf("parse outbound proxy: %w", err)
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = http.ProxyURL(proxyURL)
	return &http.Client{
		Transport: transport,
		Timeout:   followerClientTimeout(),
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}, nil
}

func parseRemoteLeaderURL(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("leader-url: %w", ErrInvalidRemoteConfig)
	}
	host := strings.ToLower(u.Hostname())
	if u.Scheme != "https" || host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", fmt.Errorf("leader-url: %w", ErrInvalidRemoteConfig)
	}
	if u.Path != "" && u.Path != "/" {
		return "", fmt.Errorf("leader-url: %w", ErrInvalidRemoteConfig)
	}
	if !strings.HasSuffix(host, ".ts.net") || host == "ts.net" || net.ParseIP(host) != nil || !validDNSName(host) {
		return "", fmt.Errorf("leader-url: %w", ErrInvalidRemoteConfig)
	}
	port := u.Port()
	if hasInvalidPort(u.Host, port) || (port != "" && !validPort(port)) {
		return "", fmt.Errorf("leader-url: %w", ErrInvalidRemoteConfig)
	}
	out := url.URL{Scheme: "https", Host: host}
	if port != "" {
		out.Host = net.JoinHostPort(host, port)
	}
	return out.String(), nil
}

func parseRemoteProxyURL(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("outbound-proxy: %w", ErrInvalidRemoteConfig)
	}
	if u.Scheme != "http" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", fmt.Errorf("outbound-proxy: %w", ErrInvalidRemoteConfig)
	}
	if u.Path != "" && u.Path != "/" {
		return "", fmt.Errorf("outbound-proxy: %w", ErrInvalidRemoteConfig)
	}
	host := u.Hostname()
	port := u.Port()
	if host != "127.0.0.1" || port == "" || hasInvalidPort(u.Host, port) || !validPort(port) {
		return "", fmt.Errorf("outbound-proxy: %w", ErrInvalidRemoteConfig)
	}
	return (&url.URL{Scheme: "http", Host: net.JoinHostPort(host, port)}).String(), nil
}

func parseRemoteMCPListen(raw string) (string, error) {
	host, port, err := net.SplitHostPort(raw)
	if err != nil {
		return "", fmt.Errorf("mcp-listen: %w", ErrInvalidRemoteConfig)
	}
	if host != "127.0.0.1" || !validPort(port) {
		return "", fmt.Errorf("mcp-listen: %w", ErrInvalidRemoteConfig)
	}
	return net.JoinHostPort(host, port), nil
}

func hasInvalidPort(hostport, parsedPort string) bool {
	return strings.Contains(hostport, ":") && parsedPort == ""
}

func validPort(raw string) bool {
	port, err := strconv.Atoi(raw)
	return err == nil && port > 0 && port <= 65535
}

func validDNSName(host string) bool {
	if len(host) == 0 || len(host) > 253 {
		return false
	}
	for _, label := range strings.Split(host, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, r := range label {
			if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
				continue
			}
			return false
		}
	}
	return true
}
