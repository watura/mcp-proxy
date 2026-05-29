# mcp-proxy

複数のMCPサーバーを1つのStreamable HTTPエンドポイントに集約するプロキシサーバー。

## 機能

- 複数のMCPサーバー (stdio / SSE / Streamable HTTP) を1つのエンドポイントに統合
- ツール・リソース・プロンプトを自動的に集約
- セッション管理とリクエストの自動振り分け

## ビルド

```bash
go build -o mcp-proxy ./cmd/mcp-proxy
```

## 使い方

```bash
mcp-proxy --config mcp.json --port 8080
```

### オプション

| フラグ | デフォルト | 説明 |
|---|---|---|
| `--config` | (必須) | mcp.json設定ファイルのパス |
| `--port` | `8080` | リッスンポート |
| `--addr` | `127.0.0.1` | リッスンアドレス |
| `--log-level` | `info` | ログレベル (debug/info/warn/error) |

## 設定ファイル (mcp.json)

プロキシが接続するバックエンドMCPサーバーを定義します。

```json
{
  "mcpServers": {
    "server-a": {
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-filesystem", "/path/to/dir"]
    },
    "server-b": {
      "url": "http://localhost:3000/mcp"
    },
    "server-c": {
      "command": "python",
      "args": ["-m", "my_mcp_server"],
      "env": {
        "API_KEY": "xxx"
      }
    }
  }
}
```

## MCPクライアントへの登録

このプロキシ自体をMCPサーバーとして登録するには、クライアント側の設定に以下のように記述します。

### stdio経由 (バイナリ直接起動)

```json
{
  "mcpServers": {
    "mcp-proxy": {
      "command": "/path/to/mcp-proxy",
      "args": ["--config", "/path/to/mcp.json"]
    }
  }
}
```

### Streamable HTTP経由 (既に起動済みのプロキシに接続)

```json
{
  "mcpServers": {
    "mcp-proxy": {
      "url": "http://127.0.0.1:8080/mcp"
    }
  }
}
```

### Claude Desktop での設定例

`~/Library/Application Support/Claude/claude_desktop_config.json`:

```json
{
  "mcpServers": {
    "mcp-proxy": {
      "command": "/path/to/mcp-proxy",
      "args": ["--config", "/path/to/mcp.json", "--port", "8080"]
    }
  }
}
```

## Docker

```bash
docker build -t mcp-proxy .
docker run -p 8080:8080 -v /path/to/mcp.json:/config/mcp.json mcp-proxy --config /config/mcp.json --addr 0.0.0.0
```
