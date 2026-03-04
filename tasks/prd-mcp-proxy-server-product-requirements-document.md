# MCP Proxy Server — Product Requirements Document

## 1. 概要

### 1.1 プロダクト名
`mcp-proxy`

### 1.2 概要
mcp.json（Claude Desktop互換形式）に定義された複数のMCPサーバーの機能を集約し、単一のプロキシサーバーとして公開するGoアプリケーション。複数のAIエージェントからの同時接続を受け付け、各エージェントごとにバックエンドMCPサーバーのプロセスを完全分離して管理する。

### 1.3 ゴール
- MCPサーバーを一元管理し、複数エージェントから共有利用可能にする
- エージェント間のセッション分離を保証する
- MCP仕様のTools / Resources / Promptsをフルサポートする

### 1.4 非ゴール
- 認証・アクセス制御（ローカル利用前提）
- バックエンドMCPサーバーの自動検出・登録
- MCPサーバーのモニタリング・メトリクス収集（v1では対象外）

---

## 2. アーキテクチャ

### 2.1 全体構成

```
┌─────────────────────────────────────────────────────┐
│                   mcp-proxy                         │
│                                                     │
│  ┌──────────────┐   ┌──────────────────────────┐   │
│  │  SSE Server   │   │  Streamable HTTP Server  │   │
│  └──────┬───────┘   └────────────┬─────────────┘   │
│         │                        │                  │
│         └──────────┬─────────────┘                  │
│                    ▼                                │
│         ┌──────────────────┐                        │
│         │  Session Manager │                        │
│         │  (per-agent)     │                        │
│         └────────┬─────────┘                        │
│                  ▼                                  │
│    ┌──────────────────────────┐                     │
│    │  Backend MCP Dispatcher  │                     │
│    │  (per-session instances) │                     │
│    └──────┬──────────┬───────┘                     │
│           │          │                              │
│     ┌─────▼────┐ ┌──▼──────┐                      │
│     │  stdio   │ │ SSE/HTTP│                       │
│     │  client  │ │ client  │                       │
│     └─────┬────┘ └──┬──────┘                      │
└───────────┼──────────┼──────────────────────────────┘
            ▼          ▼
     [MCP Server A] [MCP Server B]  ...
      (process)      (remote)
```

### 2.2 コンポーネント

| コンポーネント | 責務 |
|---|---|
| **SSE Server** | SSEトランスポートでMCPプロトコルを公開 |
| **Streamable HTTP Server** | Streamable HTTPトランスポートでMCPプロトコルを公開 |
| **Session Manager** | エージェント接続ごとにセッションを生成・管理。セッション終了時にバックエンドプロセスを破棄 |
| **Backend MCP Dispatcher** | mcp.jsonの定義に基づきバックエンドMCPサーバーへリクエストをルーティング |
| **stdio Client** | stdioベースのバックエンドMCPサーバーへの接続（プロセス起動・管理） |
| **SSE/HTTP Client** | SSE/Streamable HTTPベースのリモートMCPサーバーへの接続 |

---

## 3. 機能要件

### 3.1 mcp.json読み込み

- **フォーマット**: Claude Desktop互換（`mcpServers`キー）
- 起動引数としてmcp.jsonのパスを受け取る
- 各サーバー定義から接続方式（stdio / SSE / HTTP）を判別

```json
{
  "mcpServers": {
    "filesystem": {
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-filesystem", "/tmp"]
    },
    "remote-server": {
      "url": "http://localhost:8080/sse"
    }
  }
}
```

- `command`キーが存在 → stdioモード
- `url`キーが存在 → SSE/HTTPモード
- `env`キーが存在する場合、環境変数としてプロセスに渡す

### 3.2 フロントエンドトランスポート（エージェント向け）

#### SSE
- `/sse` エンドポイントでSSE接続を受付
- JSON-RPCメッセージをSSEイベントとして送信

#### Streamable HTTP
- `/mcp` エンドポイントでHTTPリクエストを受付
- MCP仕様のStreamable HTTPプロトコルに準拠

### 3.3 MCPプロトコル機能のプロキシ

#### Tools
- バックエンドの全MCPサーバーから`tools/list`を集約して返却
- `tools/call`リクエストを適切なバックエンドサーバーへルーティング
- ツール名の衝突時はサーバー名をプレフィックスとして付与（例: `filesystem__read_file`）
  - **衝突検出時のみ**プレフィックスを付与。衝突がないツール名はそのまま公開
  - 衝突が検出された場合、衝突する全ツールにプレフィックスを付与

#### Resources
- バックエンドの全MCPサーバーから`resources/list`を集約して返却
- `resources/read`リクエストを適切なバックエンドサーバーへルーティング
- `resources/subscribe` / `resources/unsubscribe`の転送

#### Prompts
- バックエンドの全MCPサーバーから`prompts/list`を集約して返却
- `prompts/get`リクエストを適切なバックエンドサーバーへルーティング

### 3.4 セッション管理

- 新しいエージェント接続ごとに独立したセッションを作成
- 各セッションはバックエンドMCPサーバーのプロセスを個別に起動（完全分離）
  - **stdioサーバー**: セッションごとに新プロセスをspawn
  - **リモートサーバー（SSE/HTTP）**: セッションごとに新しいクライアント接続を確立。接続レベルでの分離のみ行い、サーバー側の状態分離は保証しない
- セッション切断時にバックエンドプロセスを graceful shutdown
- セッションタイムアウト処理（設定可能なアイドルタイムアウト）

### 3.5 起動・設定

```bash
# 基本起動
mcp-proxy --config mcp.json

# ポート指定
mcp-proxy --config mcp.json --port 8080

# アドレス指定
mcp-proxy --config mcp.json --addr 0.0.0.0:8080
```

| フラグ | デフォルト | 説明 |
|---|---|---|
| `--config` | (必須) | mcp.jsonのパス |
| `--port` | `8080` | 待受ポート |
| `--addr` | `127.0.0.1` | 待受アドレス |
| `--session-timeout` | `30m` | セッションアイドルタイムアウト |
| `--log-level` | `info` | ログレベル (debug/info/warn/error) |

---

## 4. 非機能要件

### 4.1 パフォーマンス
- 同時10セッション以上をサポート
- プロキシによるレイテンシオーバーヘッドは最小限（<10ms）

### 4.2 信頼性
- バックエンドMCPサーバーのクラッシュ時にエラーをエージェントへ適切に通知
- セッション単位の障害分離（1セッションの障害が他セッションに影響しない）
- Graceful shutdown（SIGTERM/SIGINTで全セッションを適切にクリーンアップ）

### 4.3 ログ
- 構造化ログ（JSON形式）
- セッションID付きでリクエスト/レスポンスのトレーサビリティを確保
- `log/slog`を使用

---

## 5. 技術仕様

### 5.1 実装言語・主要ライブラリ

| 項目 | 選定 |
|---|---|
| **言語** | Go |
| **MCP SDK** | `github.com/mark3labs/mcp-go`（サーバー機能・クライアント機能の両方を利用） |
| **ログ** | `log/slog`（標準ライブラリ） |
| **CLI** | `flag`（標準ライブラリ） |

#### mcp-go利用方針
- **フロントエンド（エージェント向けサーバー）**: `mcp-go`のサーバー機能を利用してSSE/Streamable HTTPを公開
- **バックエンド（MCPサーバー接続）**: `mcp-go`のクライアント機能を利用してstdio/SSE/HTTPでバックエンドに接続

### 5.2 ディレクトリ構成

```
mcp-proxy/
├── cmd/
│   └── mcp-proxy/
│       └── main.go           # エントリーポイント
├── internal/
│   ├── config/
│   │   └── config.go         # mcp.json読み込み・パース
│   ├── proxy/
│   │   ├── server.go         # フロントエンドサーバー (SSE + Streamable HTTP)
│   │   ├── session.go        # セッション管理
│   │   ├── dispatcher.go     # バックエンドへのリクエストルーティング
│   │   └── aggregator.go     # Tools/Resources/Prompts集約
│   └── backend/
│       ├── stdio.go          # stdioクライアント
│       ├── sse.go            # SSEクライアント
│       └── http.go           # Streamable HTTPクライアント
├── Dockerfile
├── go.mod
├── go.sum
└── README.md
```

### 5.3 ビルド・配布

| 方式 | コマンド |
|---|---|
| **シングルバイナリ** | `go build -o mcp-proxy ./cmd/mcp-proxy` |
| **Docker** | `docker build -t mcp-proxy .` |
| **Docker実行** | `docker run -v ./mcp.json:/config/mcp.json mcp-proxy --config /config/mcp.json` |

クロスコンパイル対象:
- `linux/amd64`
- `linux/arm64`
- `darwin/amd64`
- `darwin/arm64`

---

## 6. テスト戦略

### 6.1 方針
ユニットテスト中心で各コンポーネントの単体テストを実装する。

### 6.2 テスト対象と方法

| コンポーネント | テスト方法 |
|---|---|
| **config** | mcp.jsonパース（正常系・異常系・接続方式判別） |
| **aggregator** | ツール名集約・衝突検出・プレフィックス付与ロジック |
| **dispatcher** | ツール名→バックエンドサーバーのルーティング解決 |
| **session** | セッション生成・破棄・タイムアウト処理 |
| **backend clients** | モックを利用したstdio/SSE/HTTPクライアントの動作検証 |

### 6.3 テストツール
- Go標準 `testing` パッケージ
- バックエンドMCPサーバーのモック化にはインターフェースを定義し、テスト用スタブを利用

---

## 7. エラーハンドリング

| シナリオ | 挙動 |
|---|---|
| mcp.jsonが見つからない/不正 | 起動時にエラーメッセージを出力して終了 |
| バックエンドMCPサーバー起動失敗 | セッション内でエラーをエージェントへ返却。他サーバーは影響なし |
| バックエンドMCPサーバーがクラッシュ | 該当サーバーのツール/リソースをunavailableとしてエージェントへ通知 |
| エージェント切断 | セッション内の全バックエンドプロセスをcleanup |
| ツール名衝突 | 衝突検出時のみサーバー名プレフィックスで自動解決 |

---

## 8. 将来の拡張（v2以降）

- 認証・アクセス制御（APIキー/トークン）
- バックエンドサーバーの共有モード（設定で選択可能）
- ヘルスチェック・メトリクスエンドポイント
- mcp.jsonのホットリロード
- Web管理UI
- Sampling / Roots対応