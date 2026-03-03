# Confluence CLI 設計・タスク一覧

## 前提

- **操作主体**: AI（Claude等）がCLIを操作する
- **実装方針**: TDD（テストファースト）— Red → Green → Refactor のサイクルで実装する
- **Markdown入力**: ファイルパス引数を優先、省略時は stdin から読み込む（両方サポート）
- **レイヤー構成**: service層は設けない。`cmd` が `internal/client`・`converter`・`alias`・`history` を直接オーケストレートする。コマンド横断ロジック（upsert判定・dry-run出力・history記録）は `cmd/` パッケージ内の非公開ヘルパー関数（例: `cmd/helper.go`）として分離し、コマンド間の重複を防ぐ。新パッケージは作成しない
- **インターフェース設計**: `cmd` が使うすべてのインターフェース（client / converter / alias / history）を `internal/port` に集約する。`porttest` サブパッケージにテスト用モック実装を置く（production build に混入させない）。`internal/client` 自体は `httptest` でテスト。**集約方針の根拠**: このプロジェクトはコマンド数・ドメインが確定した境界付き CLI であり、パッケージ分割のオーバーヘッドが便益を上回る。将来コマンドが大幅増加する場合はドメイン単位（page系/space系）での分割を検討する
- **ログ設計**: 独自 `logger` パッケージは設けない。Go 1.21 標準の `log/slog` を使い、`main.go` で `CONFLUENCE_CLI_LOG` が指定されていればファイルハンドラを `slog.SetDefault` で差し替える
- **コンテキスト設計**: `internal/port` の全インターフェースメソッドおよび `internal/client` の全メソッドは `ctx context.Context` を第1引数にとる。タイムアウト源泉は `cmd/root.go` に一元化する：`internal/config` は `CONFLUENCE_CLI_TIMEOUT` の読み取りと `time.ParseDuration` バリデーションのみ担い、優先順位の解決（`--timeout` フラグ > `CONFLUENCE_CLI_TIMEOUT` env > デフォルト `30s`）は `cmd/root.go` が行い `context.WithTimeout` を設定する。`cobra.Command.SetContext` で全サブコマンドに伝播させる。これによりタイムアウト統一・SIGINT 対応・テスト容易性を確保する
- **Confluence Server**: 7.9.18
- **認証方式**: PAT（Personal Access Token）
- **環境変数**:
  - `CONFLUENCE_URL` - ConfluenceのベースURL
  - `CONFLUENCE_TOKEN` - PAT
  - `CONFLUENCE_DEFAULT_SPACE` - デフォルトスペースキー
  - `CONFLUENCE_CLI_LOG` - デバッグログの出力先ファイルパス（省略時はログなし）
  - `CONFLUENCE_CLI_TIMEOUT` - コマンド全体のタイムアウト（例: `30s`、`2m`。デフォルト `30s`。`--timeout` フラグで上書き可）
- **ローカルデータ**: `~/.confluence-cli/`（`CONFLUENCE_CLI_HOME` で変更可）

---

## ディレクトリ構成

```
conflux/
├── main.go
├── cmd/                          # CLIコマンド定義
│   ├── root.go                   # ルートコマンド、--json フラグ基盤
│   ├── helper.go                 # コマンド横断ヘルパー（upsert判定・dry-run出力・history記録ラッパー）
│   ├── ping.go
│   ├── version.go
│   ├── space.go
│   ├── page.go
│   ├── attachment.go
│   ├── alias.go
│   └── history.go
├── internal/
│   ├── port/                     # cmd が共有するインターフェース集約パッケージ
│   │   ├── port.go               # SpaceClient / PageClient / AttachmentClient / Converter / AliasStore / HistoryLogger
│   │   └── porttest/             # テスト用モック（本体に混入させない）
│   │       └── mock.go           # 各インターフェースのモック実装
│   ├── client/                   # REST APIクライアント（port.SpaceClient 等を実装）
│   │   ├── client.go             # HTTPクライアント、PAT認証、リトライ
│   │   ├── space.go
│   │   ├── page.go
│   │   └── attachment.go
│   ├── apperror/                 # アプリケーションエラー（ErrorKind / ExitCode）
│   │   └── apperror.go
│   ├── config/                   # 設定管理
│   │   └── config.go             # 環境変数読み込み、バリデーション
│   ├── converter/                # Markdown↔XHTML変換（port.Converter を実装）
│   │   ├── markdown_to_xhtml.go
│   │   └── xhtml_to_markdown.go
│   ├── validator/                # 入力検証
│   │   └── validator.go
│   ├── output/                   # 出力フォーマット管理
│   │   └── output.go             # Writer 構造体、JSON/テキスト切り替え、stderr/stdout分離
│   ├── alias/                    # エイリアス管理（port.AliasStore を実装）
│   │   └── alias.go              # ~/.confluence-cli/alias.json への読み書き
│   └── history/                  # 更新履歴管理（port.HistoryLogger を実装）
│       └── history.go            # セッションID付与、history.json への記録
├── AGENTS.md
└── go.mod
```

### `~/.confluence-cli/` のファイル構成

```
~/.confluence-cli/
├── history.json    # 更新履歴
└── alias.json      # エイリアス定義
```

デバッグログは `CONFLUENCE_CLI_LOG` に指定した任意のファイルパスに出力する（固定ファイル名なし）。

### 実装順序

```
apperror → config → output → validator → port → alias → client → converter → history → cmd
```

（パッケージ間の依存関係に基づく順序。フェーズ（機能単位）の実装順序とは独立している）
（`log/slog` は標準ライブラリのため独立フェーズ不要。`main.go` でセットアップ）

### パッケージ依存関係

```
apperror  ←  すべてのパッケージ
config    ←  client, alias, history, cmd
output    ←  cmd
validator ←  cmd
port      ←  client（実装）, converter（実装）, alias（実装）, history（実装）, cmd（利用）
porttest  ←  cmd の _test.go のみ
alias     ←  cmd
client    ←  cmd
converter ←  cmd（page get / create / update のみ）
history   ←  cmd（create / update / upload のみ）
```

`cmd` がすべての `internal` パッケージを横断的に使うオーケストレーター。

---

## `history.json` スキーマ

```json
{
  "entries": [
    {
      "timestamp": "2026-03-02T10:00:00Z",
      "session_id": "abc123",
      "action": "updated",
      "page_id": "12345",
      "title": "システム概要",
      "space": "DEV",
      "version_before": 3,
      "version_after": 4
    }
  ]
}
```

- `action` は `created` / `updated` / `uploaded`（添付）の3種類（過去形で統一）
- `uploaded` アクション時は `version_before` / `version_after` フィールドを省略する
- 本文バックアップは持たない（Confluenceの版管理に任せる）
- `session_id` は1プロセス起動 = 1セッション、起動時にUUIDv4で生成
- 最大1000件保持、超過分は古いエントリから削除
- **書き込みはアトミック**: 一時ファイル（`.history.json.tmp`）に書き出してから `os.Rename()` で置き換える（クラッシュによる JSON 破損を防ぐ）
- **複数プロセス同時実行は非サポート**（単一インスタンス前提）。誤用を防ぐため AGENTS.md のフェーズ3分およびコマンドヘルプに明記する
- **書き込み失敗時ポリシー**: Confluence 操作成功後に `history.json` 書き込みが失敗した場合は **exit 0 + `WriteWarning(command, "history_write_failed", ...)`** とする。`--json` 時は stderr に warning JSON を出力、非 `--json` 時はプレーンテキスト。history はベストエフォートの補助記録であり、Confluence が真の状態管理者であるため、ローカル永続化の失敗で操作全体を失敗扱いにしない

---

## `--json` 出力スキーマ

```json
{
  "schema_version": 1,
  "command": "page get",
  "result": { ... }
}
```

- 全コマンドに `--json` オプションを提供
- **すべての `--json` 出力は `schema_version` / `command` / `result` の全ラッパーで統一する**（パーサが常に同じ構造を期待できる）
- データは stdout、ログ・警告は stderr に分離
- `result` の型: 一覧・検索系（`space list` / `page search` / `page tree` / `alias list` / `history list`）は**配列**、それ以外はオブジェクト
- `page get` のみ例外: `result` は常に配列（単一ID指定でも配列）、かつ部分失敗を示す `errors` フィールドをトップレベルに持つ

### 終了コード

| code | 意味 |
|------|------|
| `0` | 正常終了（検索結果ゼロ件・`--if-exists skip` も含む） |
| `1` | 入力検証エラー（不正なID形式、必須フラグ欠落など） |
| `2` | 認証エラー（PAT無効、権限不足） |
| `3` | ネットワーク／サーバーエラー（接続失敗、5xx、タイムアウト、**意図的中断**） |
| `4` | リソース未発見（404） |
| `5` | 競合（`--if-exists error` で既存ページあり） |

`context.DeadlineExceeded` → `kind: "timeout"` / code `3`（リトライまたはタイムアウト値の拡大を検討）。`context.Canceled` → `kind: "canceled"` / code `3`（SIGINT 等の意図的中断。リトライ不要）。どちらも code `3` だが `kind` で機械判別する。

部分成功（`page get` で一部ID失敗）は **`exit 0`** とし、`errors` フィールドに失敗分の詳細を格納する。AI は終了コードで大まかな成否を判断し、`--json` の `errors` フィールドで詳細を確認する。

### エラー時の `--json` 出力スキーマ

コマンドが失敗（exit 1–5）した場合、`WriteError` は以下を **stderr** に出力する：

```json
{
  "schema_version": 1,
  "command": "page create",
  "error": {
    "code": 4,
    "kind": "not_found",
    "message": "page 12345 not found"
  }
}
```

`kind` と `code` の対応：

| kind | code | 説明 |
|------|------|------|
| `validation_error` | `1` | 入力検証エラー |
| `auth_error` | `2` | 認証失敗 |
| `server_error` | `3` | ネットワーク障害・5xx |
| `timeout` | `3` | `context.DeadlineExceeded`。リトライ or タイムアウト拡大を検討 |
| `canceled` | `3` | `context.Canceled`（SIGINT 等）。意図的中断のためリトライ不要 |
| `not_found` | `4` | リソース未発見 |
| `conflict` | `5` | 競合 |

`page get` の部分失敗（exit 0）は `result` と同じ stdout レスポンス内の `errors` 配列に格納する（上記エラーオブジェクトに `id` フィールドを追加）。

### 警告時の `--json` 出力スキーマ

操作は成功したが付随する処理（例: history 書き込み）が失敗した場合、`WriteWarning` は以下を **stderr** に出力する（`--json` 非使用時はプレーンテキスト）：

```json
{
  "schema_version": 1,
  "command": "page create",
  "warning": {
    "kind": "history_write_failed",
    "message": "failed to write history: open ~/.confluence-cli/history.json: permission denied"
  }
}
```

現状定義する `kind` 値: `history_write_failed`（将来追加可）。

### 主要コマンドの `--json` 出力

すべてのコマンドで `schema_version` / `command` / `result` の全ラッパーを返す（以下はそれぞれの完全な出力例）。

**`ping`**
```json
{
  "schema_version": 1,
  "command": "ping",
  "result": { "ok": true, "server_version": "7.9.18", "url": "https://confluence.example.com" }
}
```

**`space list`**
```json
{
  "schema_version": 1,
  "command": "space list",
  "result": [{ "key": "DEV", "name": "開発チーム", "url": "https://..." }]
}
```

**`page search`**
```json
{
  "schema_version": 1,
  "command": "page search",
  "result": [{ "id": "12345", "title": "システム概要", "space": "DEV", "last_modified": "2026-03-01T10:00:00Z", "url": "https://..." }]
}
```

**`page get`**（`result` は常に配列、部分失敗時は `errors` をトップレベルに追加）
```json
{
  "schema_version": 1,
  "command": "page get",
  "result": [
    { "id": "12345", "title": "システム概要", "space": "DEV", "version": 4,
      "format": "markdown", "content": "...", "truncated": false, "total_chars": 1200, "url": "https://..." }
  ],
  "errors": [
    { "id": "99999", "code": 4, "kind": "not_found", "message": "page not found" }
  ]
}
```

**`page create --dry-run`**
```json
{
  "schema_version": 1,
  "command": "page create",
  "result": { "action": "preview", "preview": "<ac:structured-macro...>" }
}
```

**`page update --dry-run`**
```json
{
  "schema_version": 1,
  "command": "page update",
  "result": { "action": "preview", "diff": "--- a/page\n+++ b/page\n@@ -1 +1 @@\n-旧テキスト\n+新テキスト" }
}
```

**`page create` 通常実行**（`action` は `"created"` / `"skipped"` のいずれか）
```json
{ "schema_version": 1, "command": "page create", "result": { "action": "created", "id": "12345", "version": 1, "url": "https://..." } }
{ "schema_version": 1, "command": "page create", "result": { "action": "skipped", "id": "12345" } }
```

**`page update` 通常実行**
```json
{ "schema_version": 1, "command": "page update", "result": { "action": "updated", "id": "12345", "version": 5, "url": "https://..." } }
```

**`page tree`**（フラットリスト、ルートは `parent_id: null`）
```json
{
  "schema_version": 1,
  "command": "page tree",
  "result": [
    { "id": "100", "title": "トップページ", "parent_id": null, "depth": 0, "url": "https://..." },
    { "id": "101", "title": "子ページA",   "parent_id": "100", "depth": 1, "url": "https://..." }
  ]
}
```

**`alias list`**
```json
{
  "schema_version": 1,
  "command": "alias list",
  "result": [
    { "name": "mypage", "target": "12345", "type": "page" },
    { "name": "myspace", "target": "DEV",  "type": "space" }
  ]
}
```

**`alias get <name>`**
```json
{
  "schema_version": 1,
  "command": "alias get",
  "result": { "name": "mypage", "target": "12345", "type": "page" }
}
```

**`history list`**
```json
{
  "schema_version": 1,
  "command": "history list",
  "result": [
    { "timestamp": "2026-03-02T10:00:00Z", "session_id": "abc123",
      "action": "updated", "page_id": "12345", "title": "システム概要",
      "space": "DEV", "version_before": 3, "version_after": 4 }
  ]
}
```

**`version`**
```json
{
  "schema_version": 1,
  "command": "version",
  "result": { "version": "1.0.0", "commit": "abc1234", "built_at": "2026-03-02T10:00:00Z" }
}
```

### `internal/output` API

```go
// Writer は cmd 初期化時に1度生成して RunE に渡す。useJSON を都度引き回さない。
type Writer struct {
    JSON bool
    Out  io.Writer // デフォルト os.Stdout
    Err  io.Writer // デフォルト os.Stderr
}

func New(json bool) *Writer

// stdout に書き出す（JSON or テキスト）
func (w *Writer) Write(command string, result any) error

// stderr にエラーを書き出す（JSON or テキスト）
func (w *Writer) WriteError(command string, err error)

// stderr に警告を書き出す（JSON or テキスト）。操作は成功したが付随処理が失敗した場合に使う
func (w *Writer) WriteWarning(command string, kind string, message string)
```

`cmd/root.go` で `--json` フラグの値を受け取り `output.New(json)` で `*Writer` を生成、各サブコマンドに渡す。`Out`/`Err` を差し替えることでテストでも stdout/stderr をキャプチャできる。

---

## タスク一覧

### フェーズ1：基盤

- [ ] プロジェクト初期化（`go mod`、ディレクトリ構成）
- [ ] 設定管理 `internal/config`（`CONFLUENCE_URL`、`CONFLUENCE_TOKEN`、`CONFLUENCE_DEFAULT_SPACE`、`CONFLUENCE_CLI_TIMEOUT` の読み込みとバリデーション。`CONFLUENCE_CLI_TIMEOUT` は `time.ParseDuration` でパースし不正値はエラー、省略時は `0`（未指定）を返す。タイムアウトの優先順位解決は `cmd/root.go` の責務であり `internal/config` は関与しない）
- [ ] エラー構造体 `internal/apperror`（`ErrorKind`、`ExitCode`、終了コード一覧の定義。標準 `errors` パッケージと名前衝突しない命名）
- [ ] 出力管理 `internal/output`（`Writer` 構造体、`--json` フラグ基盤、stderr/stdout分離、`schema_version` 付与）。**契約テスト必須**（テーブルテストで3メソッド全て検証）: ①`Write` が `{"schema_version":1,"command":"...","result":...}` を stdout に出力する、②`WriteError` が `{"schema_version":1,"command":"...","error":{"code":N,"kind":"...","message":"..."}}` を stderr に出力する、③`WriteWarning` が `{"schema_version":1,"command":"...","warning":{"kind":"...","message":"..."}}` を stderr に出力する。非 `--json` 時はプレーンテキスト出力になることも検証する
- [ ] 入力検証 `internal/validator`（APIを叩く前のID形式・スペースキー検証）
- [ ] インターフェース集約 `internal/port`（`SpaceClient`/`PageClient`/`AttachmentClient`/`Converter`/`AliasStore`/`HistoryLogger`）+ `internal/port/porttest`（各インターフェースのモック実装。production build に含まない）
- [ ] REST APIクライアント `internal/client`（`port.SpaceClient`/`PageClient`/`AttachmentClient` を実装、PAT認証、指数バックオフリトライ・最大3回。**読み取り系（GET）は 429/5xx で再試行**。**書き込み系（POST/PUT）は 429 のみ再試行**――5xx は処理が完了している可能性があるため再試行せず、重複作成・二重更新を防ぐ。**429 再試行時は `Retry-After` ヘッダ値を優先**し、ヘッダ未提供時はジッター付き指数バックオフ（ベース 1s、最大 8s）を使用。テスト観点: 連続 429 でも `Retry-After` を上限に待機し、3回超で諦めることを `httptest` で確認）
- [ ] `log/slog` セットアップ（`main.go` で `CONFLUENCE_CLI_LOG` が指定されていればファイルハンドラを `slog.SetDefault` で差し替え）
- [ ] `ping` コマンド（疎通確認）
- [ ] `version` コマンド
- [ ] AGENTS.md（フェーズ1分：環境変数一覧、終了コード一覧、エラー種別一覧、`--json` スキーマ仕様）

### フェーズ2：閲覧系

- [ ] 変換レイヤー `internal/converter`（`port.Converter` を実装。ライブラリ: GFM→XHTML は `goldmark`、XHTML→GFM は `goquery` ベースの独自実装；Confluenceマクロは変換対象外とし `<!-- macro:... -->` コメントとして保持；`--section` 向けのXHTMLセクション抽出も本パッケージの責務）※実装の最初にテーブル・コードブロック・Confluenceマクロ等の主要パターンを試作して変換方針を確定する
- [ ] `space list`
- [ ] `page search [keyword] --space --after` - キーワードは位置引数（省略可）、スペース＋最終更新日でフィルタ（`--space` は省略可、省略時はデフォルトスペース、それもなければ全スペース。`--after` は `YYYY-MM-DD` 形式）
- [ ] `page get <ID...> --format markdown|html|storage --section <SECTION_ID> --max-chars` - IDは空白区切りで複数指定可。`--format` デフォルトは `markdown`（XHTML→GFM変換に converter を使用）、`--format storage` で生XHTML取得可。`--section` 指定時は converter がアンカーIDで該当セクションを抽出してから変換（API はセクション単位取得不可）。`--max-chars` は段落単位で切り、`result` に `"truncated": true` と `"total_chars": N` を付与。**回帰テスト必須**: 複数ID指定で一部が404の場合に exit 0 かつ `result[]` に成功分・`errors[]` に失敗分が入ることをテーブルテストで検証
- [ ] `page tree --space --depth` - 深さ制限付きページツリー取得（デフォルト `--depth 3`、最大10）、出力はフラットリスト＋`parent_id`（ルートページは `"parent_id": null`）※フェーズ2最後に実装
- [ ] `attachment list <ID>`
- [ ] `alias set/get/list/delete` - `port.AliasStore` を実装。ページIDやスペースキーに短縮名を付けて複数コマンドをまたいで使い回す（`alias set <name> <target> [--type page|space]`、`--type` デフォルトは `page`）
- [ ] AGENTS.md（フェーズ2分：各コマンドの入出力スキーマ、エイリアスの使い方）

### フェーズ3：編集系

- [ ] `page create [file] --space --title --dry-run --if-exists skip|error|update` - Markdownはファイルパス引数または stdin から読み込む。**タイトルは `--title` で指定（省略時は Markdown 先頭の `# heading` から取得、それもなければエラー）**。`--space` は必須（省略かつデフォルトスペースなしはエラー）、`--dry-run` 時は変換後XHTMLを `result.preview` に格納して stdout に出力（アップロードなし）。冪等性対応：`--if-exists update` はタイトル＋スペースキーで CQL 検索し、**0件→新規作成、1件→更新、2件以上→exit 5（"ambiguous: N pages with same title in space X"）**。通常実行時の `result.action` は `"created"` / `"skipped"` / `"updated"`。**テーブルテスト必須**: `--if-exists update` で検索結果が 0件/1件/2件以上の3ケースをモックを使いテーブルドリブンで検証（期待 action と exit code を明示）
- [ ] `page update <ID> [file] --dry-run` - Markdownはファイルパス引数または stdin から読み込み、XHTMLに変換してアップロード。`--dry-run` 時は unified diff を `result.diff` に格納して stdout に出力（アップロードなし）。通常実行時は `result` に `"action": "updated"` と `"version_after": N` を付与
- [ ] `attachment upload <page-ID> <file>` - 指定ページにファイルを添付アップロード
- [ ] `attachment download <attachment-ID> --output` - `<attachment-ID>` は `attachment list` で取得する添付ファイルID。保存先指定（デフォルトはカレントディレクトリ）。`--output -` で stdout に出力し、パイプライン連携を可能にする
- [ ] 更新履歴 `internal/history`（`port.HistoryLogger` を実装。`~/.confluence-cli/history.json` への記録、セッションID付与。書き込みは temp+rename でアトミックに行う。**書き込み失敗は呼び出し元にエラーを返すのみ**――`cmd` 側で exit 0 + stderr warning に変換する責務を持つ）
- [ ] `history list --limit --space --session` - 更新履歴の表示、セッション単位フィルタ
- [ ] AGENTS.md（フェーズ3分：editフロー、dry-run確認ルール、historyスキーマ、パイプ連携ユースケース例、冪等性の挙動、エラーコード一覧）

---

## コマンド一覧まとめ

| コマンド | 説明 |
|---|---|
| `ping` | 疎通確認 |
| `version` | バージョン表示 |
| `space list` | スペース一覧 |
| `page search` | ページ検索 |
| `page get <ID...>` | ページ内容取得（複数ID対応、`--section <SECTION_ID>` でセクション指定） |
| `page tree` | ページツリー取得 |
| `page create` | ページ新規作成 |
| `page update <ID>` | ページ更新 |
| `attachment list <ID>` | 添付ファイル一覧 |
| `attachment upload <page-ID> <file>` | 添付ファイルアップロード |
| `attachment download <attachment-ID>` | 添付ファイルダウンロード（IDは `attachment list` で取得） |
| `alias set/get/list/delete` | エイリアス管理 |
| `history list` | 更新履歴表示 |

> **非実装**: `page delete` / `page move` は安全策として意図的に対象外とする。削除・移動はConfluence UIで行う。
