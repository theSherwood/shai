# MCP Alias Server

Shai now exposes host calls over a lightweight JSON-RPC endpoint that follows the Model Context Protocol (MCP) tool schema. The server runs on the host and is reachable from the container via `host.docker.internal`.

## Environment Variables

The runner injects the following vars whenever calls are present:

| Variable | Description |
| --- | --- |
| `SHAI_ALIAS_ENDPOINT` | HTTP endpoint (e.g. `http://host.docker.internal:34567/mcp`) |
| `SHAI_ALIAS_TOKEN` | Bearer token required for every request |
| `SHAI_ALIAS_SESSION_ID` | Unique identifier for the alias session |

Containers should include the `Authorization: Bearer ${SHAI_ALIAS_TOKEN}` header on every request.

## API Shape

All requests use JSON-RPC 2.0 over HTTP `POST` to `${SHAI_ALIAS_ENDPOINT}`.

### `listTools`

Request:

```json
{"jsonrpc":"2.0","id":1,"method":"listTools"}
```

Response:

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "tools": [
      {
        "name": "git-sync",
        "description": "Runs git pull --rebase",
        "inputSchema": {
          "type": "object",
          "properties": {
            "args": { "type": "array", "items": { "type": "string" } }
          }
        }
      }
    ]
  }
}
```

### `callTool`

Request:

```json
{
  "jsonrpc": "2.0",
  "id": 2,
  "method": "callTool",
  "params": {
    "name": "git-sync",
    "args": ["--dry-run"]
  }
}
```

Successful response:

```json
{
  "jsonrpc": "2.0",
  "id": 2,
  "result": {
    "exitCode": 0,
    "content": [
      {"type":"text","stream":"stdout","text":"up to date\n"}
    ]
  }
}
```

Errors return a standard JSON-RPC error object (e.g. argument validation failures or unknown calls). Stdout/stderr data is returned as text chunks with a `stream` field identifying the source.
