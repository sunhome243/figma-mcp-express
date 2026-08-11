# Remote follower (beta): private Tailnet setup

Remote follower is **beta**. Use it only when Figma Desktop remains on a leader Mac and the AI client runs elsewhere in the same private Tailnet.

## Agent rules

- Do not configure Tailscale, create tunnels, enable Funnel, or bind a server to a network interface. Ask the user to complete the host/runtime setup.
- The MCP client connects to `http://127.0.0.1:<follower-port>/mcp` in its own runtime. Never send MCP calls to the leader `.ts.net` URL; it exposes the bridge, not an MCP endpoint.
- Treat Tailnet ACLs as the access boundary. This mode is not a public endpoint and does not supply OAuth user authorization.
- Before any Figma write, require a successful `GET /readyz`, then call `get_metadata`. On failure, ask the user to check the outbound HTTP proxy, `tailscale serve status --json`, then the Figma plugin channel.

## Required user-owned values

1. Leader HTTPS root URL from `tailscale serve status --json`.
2. A loopback-only outbound HTTP proxy in the AI runtime.
3. A free loopback follower port, for example `45124`.

The user starts:

```bash
figma-mcp-express --mode remote-follower \
  --leader-url https://<leader>.<tailnet>.ts.net \
  --outbound-proxy http://127.0.0.1:<proxy-port> \
  --mcp-listen 127.0.0.1:45124
```

For Codex registration use `codex mcp add figma-mcp-express-remote --url http://127.0.0.1:45124/mcp`.
