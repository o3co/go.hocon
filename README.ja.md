# go.hocon

[![Go Reference](https://pkg.go.dev/badge/github.com/o3co/go.hocon.svg)](https://pkg.go.dev/github.com/o3co/go.hocon)
[![Go Report Card](https://goreportcard.com/badge/github.com/o3co/go.hocon)](https://goreportcard.com/report/github.com/o3co/go.hocon)
[![CI](https://github.com/o3co/go.hocon/actions/workflows/test.yml/badge.svg)](https://github.com/o3co/go.hocon/actions/workflows/test.yml)
[![Lint](https://github.com/o3co/go.hocon/actions/workflows/lint.yml/badge.svg)](https://github.com/o3co/go.hocon/actions/workflows/lint.yml)
[![License](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](LICENSE)

[Lightbend HOCON](https://github.com/lightbend/config/blob/main/HOCON.md) 仕様に完全準拠した Go ライブラリ。

> **[Claude](https://claude.ai/)（Anthropic）による実装** — 設計・実装のすべてを Claude Code が担当。

[English](README.md)

---

## 特徴

- HOCON の全構文をサポート：オブジェクト、配列、スカラー、代入（`${path}`、`${?path}`）
- 自己参照代入（`path = ${path} ["/extra"]`）
- 重複キーのディープマージ（後勝ちセマンティクス）
- `+=` 追記演算子
- `include "file.conf"` および `include file("file.conf")` ディレクティブ
- トリプルクォート文字列（`"""..."""`）
- Duration パース（`10ms`、`2s`、`1h`、`1d`）
- バイトサイズパース（`1KB`、`1KiB`、`1MB`、…）
- 安全な省略値アクセスのためのジェネリック `Option[T]`
- `hocon` 構造体タグによる Unmarshal
- 外部依存ゼロ — 標準ライブラリのみ

## インストール

```bash
go get github.com/o3co/go.hocon
```

Go 1.21 以上が必要。

## クイックスタート

```go
import "github.com/o3co/go.hocon"

// 文字列からパース
cfg, err := hocon.ParseString(`
  server {
    host = "localhost"
    port = 8080
    timeout = "30s"
  }
`)

// ファイルからパース
cfg, err = hocon.ParseFile("application.conf")

// スカラーゲッター（missing/型違い時はパニック）
host    := cfg.GetString("server.host")        // "localhost"
port    := cfg.GetInt("server.port")           // 8080
timeout := cfg.GetDuration("server.timeout")   // 30 * time.Second

// Option 系（パニックしない安全版）
host := cfg.GetStringOption("server.host").OrElse("localhost")
port := cfg.GetInt64Option("server.port").OrElse(8080)
```

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

[Lightbend 公式テストスイート](https://github.com/lightbend/config/tree/main/config/src/test/resources) で検証済み：**13 グループ中 12 グループが PASS**。スキップした 1 グループ（`equiv03/includes.conf`）は `.properties` ファイルのパースが必要で、v1.0 のスコープ外。

## ライセンス

Apache License 2.0 — [LICENSE](LICENSE) 参照。

Copyright 2026 o3co Inc.
