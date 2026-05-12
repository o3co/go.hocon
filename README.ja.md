# go.hocon — Go 向け HOCON パーサー

[![Go Reference](https://pkg.go.dev/badge/github.com/o3co/go.hocon.svg)](https://pkg.go.dev/github.com/o3co/go.hocon)
[![Go Report Card](https://goreportcard.com/badge/github.com/o3co/go.hocon)](https://goreportcard.com/report/github.com/o3co/go.hocon)
[![CI](https://github.com/o3co/go.hocon/actions/workflows/test.yml/badge.svg)](https://github.com/o3co/go.hocon/actions/workflows/test.yml)
[![Lint](https://github.com/o3co/go.hocon/actions/workflows/lint.yml/badge.svg)](https://github.com/o3co/go.hocon/actions/workflows/lint.yml)
[![codecov](https://codecov.io/gh/o3co/go.hocon/branch/main/graph/badge.svg)](https://codecov.io/gh/o3co/go.hocon)
[![License](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](LICENSE)

[Lightbend HOCON](https://github.com/lightbend/config/blob/main/HOCON.md) 仕様の Go パーサー。現在の準拠率は [仕様準拠](#仕様準拠) を参照。

> **[Claude](https://claude.ai/)（Anthropic）による実装** — 設計・実装のすべてを Claude Code が担当。
> [GitHub Copilot](https://github.com/features/copilot) および [OpenAI Codex](https://openai.com/index/openai-codex/) によるレビュー。

[English](README.md)

---

## クイックスタート

### 1. インストール

```bash
go get github.com/o3co/go.hocon
```

Go 1.21 以上が必要。

### 2. 使い方

```go
import "github.com/o3co/go.hocon"

cfg, err := hocon.ParseString(`
  server {
    host = "localhost"
    port = 8080
  }
`)
if err != nil {
    log.Fatal(err)
}

host := cfg.GetString("server.host")  // "localhost"
port := cfg.GetInt("server.port")     // 8080
```

## なぜ HOCON？

| | `.env` | JSON | YAML | HOCON |
|---|---|---|---|---|
| Comments | No | No | Yes | Yes |
| Nesting | No | Yes | Yes | Yes |
| References / Substitution | No | No | No | Yes (`${var}`) |
| File inclusion | No | No | No | Yes (`include`) |
| Object merging | No | No | Anchors (fragile) | Yes (deep merge) |
| Optional values | No | No | No | Yes (`${?var}`) |
| Trailing commas | N/A | No | N/A | Yes |
| Unquoted strings | Yes | No | Yes | Yes |

HOCON は YAML の可読性と JSON の構造性を兼ね備え、どちらにもない機能 — 変数参照、インクルード、ディープマージ — を提供します。設定がフラットなキーバリューペア以上のものであれば、HOCON を検討する価値があります。

## 特徴

- HOCON の全構文をサポート：オブジェクト、配列、スカラー、代入（`${path}`、`${?path}`）
- 自己参照代入（`path = ${path} ["/extra"]`）
- 重複キーのディープマージ（後勝ちセマンティクス）
- `+=` 追記演算子
- `include "file.conf"` および `include file("file.conf")` ディレクティブ
- トリプルクォート文字列（`"""..."""`）
- Duration パース（`10ms`、`2s`、`1h`、`1d`）
- バイトサイズパース（`1KB`、`1KiB`、`1MB`、...）
- 安全な省略値アクセスのためのジェネリック `Option[T]`
- `hocon` 構造体タグによる Unmarshal
- 外部依存ゼロ — 標準ライブラリのみ

## API

### パース

```go
hocon.ParseString(input string) (*Config, error)
hocon.ParseFile(path string)    (*Config, error)
```

### スカラーゲッター

| メソッド | 戻り値 | パニック条件 |
|---------|-------|------------|
| `GetString(path)` | `string` | missing・null・型違い |
| `GetInt(path)` | `int` | missing・null・型違い |
| `GetInt64(path)` | `int64` | missing・null・型違い |
| `GetFloat64(path)` | `float64` | missing・null・型違い |
| `GetFloat32(path)` | `float32` | missing・null・型違い |
| `GetBool(path)` | `bool` | missing・null・型違い |
| `GetDuration(path)` | `time.Duration` | missing・null・不正フォーマット |
| `GetBytes(path)` | `int64` | missing・null・不正フォーマット |

それぞれに `GetXxxOption(path) Option[T]` 版があり、パニックの代わりに `None` を返す。

### スライスゲッター

```go
cfg.GetStringSlice(path)   []string
cfg.GetInt64Slice(path)    []int64
cfg.GetIntSlice(path)      []int
cfg.GetConfigSlice(path)   []*Config
```

それぞれに `GetXxxSliceOption` 版あり。

### オブジェクトアクセス

```go
sub := cfg.GetConfig("server")        // "server" スコープの *Config
opt := cfg.GetConfigOption("server")  // Option[*Config]
```

### 検査

```go
cfg.Has("server.host")  // null 値でも true
cfg.Keys()              // 直接の子キー一覧（宣言順）
```

### フォールバックマージ

```go
merged := overrides.WithFallback(defaults)
// overrides が優先。defaults は不足キーを補完する
```

### Option[T]

```go
opt := cfg.GetStringOption("key")
if opt.IsSome() {
    v, _ := opt.Get()
}
v := opt.OrElse("default")
```

### Unmarshal

```go
type ServerConfig struct {
    Host    string        `hocon:"host"`
    Port    int           `hocon:"port"`
    Timeout time.Duration `hocon:"timeout,omitempty"`
    Tags    []string      `hocon:"tags"`
}

var s ServerConfig
err := cfg.Unmarshal(&s)

// map[string]any も対応
m := make(map[string]any)
err = cfg.Unmarshal(&m)
```

`hocon` タグがないフィールドはフィールド名を小文字化したキーで検索する。`omitempty` はキーが存在しないとき、フィールドの既存値を保持する。

### エラー型

```go
var pe *hocon.ParseError   // 字句解析・構文解析エラー — Line, Col, FilePath を持つ
var re *hocon.ResolveError // 代入・include 解決エラー — Path を持つ
var ce *hocon.ConfigError  // GetXxx パニックのペイロード — Path を持つ
```

## HOCON の例

```hocon
# コメントは # または //
database {
  host = "db.example.com"
  port = 5432
  url  = "jdbc:"${database.host}":"${database.port}  // 代入 + 文字列連結
}

# 重複キーはディープマージ（スカラーは後勝ち）
server { host = localhost }
server { port = 8080 }      // 結果: { host: localhost, port: 8080 }

# 自己参照追記
path = ["/usr/bin"]
path = ${path} ["/usr/local/bin"]  // ["/usr/bin", "/usr/local/bin"]

# += 演算子
items = [1]
items += [2, 3]   // [1, 2, 3]

# インクルード
include "defaults.conf"
include file("overrides.conf")

# Duration・バイトサイズ
timeout   = "30s"
cache-ttl = "5m"
max-size  = "512MiB"
```

## 仕様準拠

[Lightbend HOCON 仕様](https://github.com/lightbend/config/blob/main/HOCON.md) への準拠状況は [`docs/spec-compliance.md`](docs/spec-compliance.md) に項目単位で記載しています。以下の表は 2026-05-12 時点のスナップショットです — 最新値は [`xx.hocon/docs/compliance-matrix.md`](https://github.com/o3co/xx.hocon/blob/main/docs/compliance-matrix.md) を参照してください。

| 指標 | 状況 |
| --- | --- |
| 仕様全体（out-of-scope を含む） | **61.0%** |
| In-scope のみ | **79.7%** |
| Lightbend `equiv01`–`equiv05` + `test01`–`test13` | 13/13 合格 |
| [hocon2](https://github.com/o3co/hocon2) 準拠テスト（JSON/YAML/TOML/Properties 出力） | 77/77 合格 |

## 関連プロジェクト

| プロジェクト | 言語 | レジストリ | 説明 |
|---------|----------|----------|-------------|
| [ts.hocon](https://github.com/o3co/ts.hocon) | TypeScript | [npm](https://www.npmjs.com/package/@o3co/ts.hocon) | TypeScript/Node.js 向け HOCON パーサー |
| [rs.hocon](https://github.com/o3co/rs.hocon) | Rust | [crates.io](https://crates.io/crates/o3co-hocon) | Rust 向け HOCON パーサー |
| [hocon2](https://github.com/o3co/hocon2) | Go | [pkg.go.dev](https://pkg.go.dev/github.com/o3co/hocon2) | HOCON → JSON/YAML/TOML/Properties 変換 CLI |

3 つのパーサー実装（[ts.hocon](https://github.com/o3co/ts.hocon)、[rs.hocon](https://github.com/o3co/rs.hocon)、[go.hocon](https://github.com/o3co/go.hocon)）はすべて同じ Lightbend HOCON 仕様で追跡されています — 実装ごとの準拠率は [横断ロールアップ](https://github.com/o3co/xx.hocon/blob/main/docs/compliance-matrix.md) を参照してください。

## ベストプラクティス

### 設定構成

- **ドメインごとに分割**: 設定を論理的な単位に分けましょう（`database.conf`、`server.conf`、`logging.conf`）
- **`include` で合成**: ドメイン別ファイルからフル設定を組み立てましょう
- **設定にロジックを入れない**: HOCON は宣言的なデータのためのもので、条件分岐や計算には向きません

### 環境変数

- **`${ENV}` の使用を最小限に**: 設定ファイル自体にデフォルト値を定義し、`${?ENV}`（オプショナル）を使いましょう
- **ローカル開発で環境変数を必須にしない**: デフォルトだけで動くようにしましょう
- **必須の環境変数を文書化**: プロジェクトの README や `.env.example` にリストしましょう

### 開発 / 本番の分離

```text
config/
├── application.conf    # 共有デフォルト
├── dev.conf            # include "application.conf" + 開発用オーバーライド
└── prod.conf           # include "application.conf" + 本番用オーバーライド
```

### バリデーション

- 設定のバリデーションは常にアプリケーション起動時に行い、使用時ではなく早期に検出しましょう
- スキーマバリデーション（TypeScript は Zod、Go は struct Unmarshal、Rust は Serde）を使って早期にエラーをキャッチしましょう

```go
conf, err := hocon.ParseString(`
server {
  host = "localhost"
  port = 8080
}
debug = true
`)
if err != nil {
    log.Fatal(err)
}

var app struct {
    Server struct { Host string; Port int } `hocon:"server"`
    Debug  bool                             `hocon:"debug"`
}
if err := conf.Unmarshal(&app); err != nil {
    log.Fatal(err) // 起動時に即座に失敗
}
```

## ライセンス

Apache License 2.0 — [LICENSE](LICENSE) を参照。

Copyright 2026 1o1 Co. Ltd.
