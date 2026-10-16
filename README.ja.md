# go.hocon — Go 向け HOCON パーサー

[![Go Reference](https://pkg.go.dev/badge/github.com/o3co/go.hocon.svg)](https://pkg.go.dev/github.com/o3co/go.hocon)
[![Go Report Card](https://goreportcard.com/badge/github.com/o3co/go.hocon)](https://goreportcard.com/report/github.com/o3co/go.hocon)
[![CI](https://github.com/o3co/go.hocon/actions/workflows/test.yml/badge.svg)](https://github.com/o3co/go.hocon/actions/workflows/test.yml)
[![Lint](https://github.com/o3co/go.hocon/actions/workflows/lint.yml/badge.svg)](https://github.com/o3co/go.hocon/actions/workflows/lint.yml)
[![codecov](https://codecov.io/gh/o3co/go.hocon/branch/develop/graph/badge.svg)](https://codecov.io/gh/o3co/go.hocon)
[![License](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](LICENSE)

[Lightbend HOCON](https://github.com/lightbend/config/blob/main/HOCON.md) 仕様の Go パーサー。現在の準拠率は [仕様準拠](#仕様準拠) を参照。

> **[Claude](https://claude.ai/)（Anthropic）による実装** — 設計・実装のすべてを Claude Code が担当。
> [GitHub Copilot](https://github.com/features/copilot) および [OpenAI Codex](https://openai.com/index/openai-codex/) によるレビュー。

**ドキュメント:** [o3co.github.io/hocon](https://o3co.github.io/hocon/) — 4 実装・hocon2 CLI・共通の準拠コーパスをまとめた横断ハブです。

[English](README.md)

**ライブラリの立ち位置** — go.hocon は低レベルのパーサー API ではなく、HOCON の設定ローダーです。目的は `.hocon` ファイルを読み、Config API（`GetString`、`GetInt`、`GetFloat64`、`GetBool`、`GetDuration`、`GetBytes`、`Unmarshal`）で型付きにアクセスできるようにすることです。`ScalarVal` などの内部型はマイナーバージョン間で変わることがあります。

**言語横断の準拠テスト** — この実装は [o3co/xx.hocon](https://github.com/o3co/xx.hocon) の共有期待値 JSON フィクスチャで、[ts.hocon](https://github.com/o3co/ts.hocon)、[rs.hocon](https://github.com/o3co/rs.hocon)、[py.hocon](https://github.com/o3co/py.hocon) と並べてテストされており、4 実装すべてが同じ Lightbend HOCON 仕様を満たすことを確認しています。

---

## クイックスタート

### 1. インストール

```bash
go get github.com/o3co/go.hocon
```

Go 1.23 以上が必要です。

### 2. 使い方

```go
import "github.com/o3co/go.hocon"

type App struct {
    Server struct {
        Host string `hocon:"host"`
        Port int    `hocon:"port"`
    } `hocon:"server"`
}

cfg, err := hocon.ParseString(`
  server {
    host = "localhost"
    port = 8080
  }
`)
if err != nil {
    log.Fatal(err)
}

var app App
if err := cfg.Unmarshal(&app); err != nil {
    log.Fatal(err) // 必須フィールドの欠落・型違いは起動時に fail する
}
// app.Server.Host == "localhost", app.Server.Port == 8080

// 単一の値は Get*E が (T, error) を返す
host, err := cfg.GetStringE("server.host")
if err != nil {
    log.Fatal(err)
}
```

既定では `Unmarshal` (struct 丸ごと、起動時に fail fast) か、error を返す
`Get*E` ゲッターを使ってください。panic する `Get*` と `Option` を返す
`Get*Option` は、その意味論が合う場面向けの variant です —
[スカラーゲッター](#スカラーゲッター) を参照。

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

HOCON は単なるシリアライズ形式ではなく、**プログラムに注入するための設定言語** です。JSON / YAML / TOML はデータ構造の表現に徹しており、ファイルの重ね合わせ・環境変数・参照解決はアプリ側（Pydantic、Serde、Zod 等）の責務になります。HOCON はそれらを仕様そのものに内包しているため、プログラムが設定を受け取る時点で、フォールバックは合成済み・`${VAR}` 参照は解決済みの「1 枚の設定」になっています。「このレイヤーに値があるか？」に由来する条件分岐は、コードではなくフォーマット境界で消えます。

加えて HOCON は YAML の可読性と JSON の構造性を兼ね備えるため、フラットなキーバリュー設定を超えるユースケースには強い選択肢になります。

## モジュールの中に閉じる設定

大規模なコードベースでは、設定が漏れ出しがちです。全チームが編集する巨大な 1 ファイル、サービスごとにコピーされたデフォルト値、互いのキーを読み合うモジュール。HOCON では、各モジュールがコードと同じように自分の設定を所有できます。

- **各モジュールが名前空間を所有し、自分のデフォルトを同梱する。** `auth` モジュールは `auth { … }` を自分のコードの隣に置き、すべてのデフォルト値を埋めておきます。アプリケーションはそのデフォルトをコピーせず、`include` 1 行で取り込みます。
- **アプリケーションは違う部分だけを上書きする。** 後のレイヤーが前のレイヤーにディープマージされるので、アプリケーションが書くのはブロック全体ではなく 2 キーだけです。
- **各モジュールは自分のサブツリーだけを、自分のスキーマで読む。** モジュールは `auth` を要求し、自分が定義したスキーマで検証します。そのセクションの形がモジュールの契約で、ファイルの他の部分はモジュールから見えません。
- **モジュール間の配線はアプリケーションに置く。** モジュールは自分が読むキー（`auth.db`）を宣言するだけで、アプリケーションが与えるべきものにはデフォルトを置きません。アプリケーションがそれを共有の値につなぎます（`auth.db = ${shared.db}`）。モジュールは互いのキーもアプリケーションのキーも参照しないので、依存は一方向になります。

```hocon
# auth/reference.conf: auth パッケージに同梱、auth チームが所有
auth {
  session { lifetime = 30m, max-body = 64KiB }
  # db: デフォルトなし。アプリケーションが auth.db を与える
}
```

```hocon
# application.conf: アプリケーションが所有
include package("github.com/acme/auth", "reference.conf")

shared.db { host = db.internal, port = 5432 }
auth.db = ${shared.db}       # 配線はアプリケーションに置く
auth.session.lifetime = 2h   # 違う部分だけ上書きし、残りはモジュールのデフォルトのまま
```

```go
// auth モジュールの中
package auth

import (
    _ "embed"
    "fmt"
    "time"

    "github.com/o3co/go.hocon"
)

//go:embed reference.conf
var reference []byte

func init() {
    // デフォルトをモジュールの識別子で公開する
    if err := hocon.RegisterPackage("github.com/acme/auth", "reference.conf", reference); err != nil {
        panic(err)
    }
}

// Auth はモジュールの契約: auth セクションの形
type Auth struct {
    Session struct {
        Lifetime time.Duration  `hocon:"lifetime"`
        MaxBody  hocon.ByteSize `hocon:"max-body"`
    } `hocon:"session"`
    DB struct {
        Host string `hocon:"host"`
        Port int    `hocon:"port"`
    } `hocon:"db"` // デフォルトなし: アプリケーションが auth.db を与える
}

// Load は auth のサブツリーだけを読む
func Load(root *hocon.Config) (Auth, error) {
    var auth Auth
    if err := root.UnmarshalPath("auth", &auth); err != nil {
        return auth, err
    }
    if auth.Session.Lifetime <= 0 {
        return auth, fmt.Errorf("auth.session.lifetime must be positive, got %v", auth.Session.Lifetime)
    }
    return auth, nil
}
```

`include package(...)` は HOCON.md の一部ではなく、o3co の実装が共有する規約です（[E11](https://github.com/o3co/xx.hocon/blob/main/docs/extra-spec-conventions.md#e11)）。`include classpath(...)` に相当する、JVM 以外のための仕組みです。アプリケーションがファイルの置き場所を知らなくても、モジュールはこれでデフォルトを公開できます（[パッケージ include](#パッケージ-include-include-package) を参照）。JVM では Lightbend の `reference.conf` 規約が同じ役割を担います。

## 特徴

- HOCON の全構文をサポート：オブジェクト、配列、スカラー、代入（`${path}`、`${?path}`）
- 自己参照代入（`path = ${path} ["/extra"]`）
- 重複キーのディープマージ（後勝ちセマンティクス）
- `+=` 追記演算子
- `include "file.conf"` および `include file("file.conf")` ディレクティブ
- トリプルクォート文字列（`"""..."""`）
- Duration パース（`10ms`、`2s`、`1h`、`1d`）
- バイトサイズパース（`1kB`、`1KiB`、`1MB`、...）
- 期間 (Period) パース（`7d`、`2w`、`3mo`、`1y`）→ `hocon.Period`
- 安全な省略値アクセスのためのジェネリック `Option[T]`
- `hocon` 構造体タグによる Unmarshal
- 遅延解決: 実行時に渡す代入値のための `Parse` / `WithFallback` / `Resolve` の分離されたライフサイクル（v1.4.0+）
- インメモリのフォールバック層を作る `FromMap` / `Empty`（v1.4.0+）
- 他プログラムが所有する設定ファイルを読むフォーマットアダプタ — Properties、env、JSONC、JSON5、TOML、YAML（下記参照）
- 実行時の外部依存なし — `gopkg.in/yaml.v3` はテストコード（フィクスチャのシナリオランナー）でのみ使用

## API

### パース

```go
hocon.ParseString(input string) (*Config, error)
hocon.ParseFile(path string)    (*Config, error)

// 遅延パース (v1.4.0+): 入力に `${...}` があり、解決前に WithFallback で
// フォールバック値を重ねたいときは、未解決の Config を返す。
hocon.ParseStringWithOptions(input string, opts hocon.ParseOptions) (*Config, error)
hocon.ParseFileWithOptions(path string,    opts hocon.ParseOptions) (*Config, error)
hocon.FromMap(values map[string]any, originDescription string)     (*Config, error)
hocon.Empty(originDescription string)                              *Config
```

#### 遅延解決の例 (v1.4.0+)

代入が実行時にしか得られない値（CI のビルド番号など）を参照するときに使います — [#99](https://github.com/o3co/go.hocon/issues/99) を参照。

```go
cfg, _ := hocon.ParseStringWithOptions(
    `version = ${shortversion}-${CI_RUN_NUMBER}
     variables { shortversion = "1.2.3" }`,
    hocon.DefaultParseOptions().WithResolveSubstitutions(false),
)
runtime, _ := hocon.FromMap(map[string]any{"CI_RUN_NUMBER": "42"}, "")
vars := cfg.GetConfig("variables")
resolved, _ := cfg.
    WithFallback(runtime).
    WithFallback(vars).
    Resolve(hocon.DefaultResolveOptions())
fmt.Println(resolved.GetString("version")) // "1.2.3-42"
```

`ResolveOptions.WithUseSystemEnvironment(false)` で環境変数を参照しない閉じた解決（`os.Environ` へのフォールバックなし）に、`.WithAllowUnresolved(true)` で部分解決（未解決のパスは placeholder のまま残り、そこへの `Get*E` は `errors.Is(err, hocon.ErrNotResolved)` を満たすエラーを返す）になります。

### スカラーゲッター

3 系列は同じパス解決・型変換を共有し、違いは「missing / null / 型違い」の
返し方だけです。**アプリケーションコードでは error を返す `Get*E` 系列を
既定に**してください。panic 系列は検証済みの config (例: `main` で
`Unmarshal` 直後) 向け、`Option` 系列は default 付きの任意キー
(`OrElse`) 向けです。

| Error 返却 (推奨) | Panic | Option |
|---|---|---|
| `GetStringE(path) (string, error)` | `GetString(path) string` | `GetStringOption(path) Option[string]` |
| `GetIntE` / `GetInt64E` | `GetInt` / `GetInt64` | `GetIntOption` / `GetInt64Option` |
| `GetFloat64E` / `GetFloat32E` | `GetFloat64` / `GetFloat32` | `GetFloat64Option` / `GetFloat32Option` |
| `GetBoolE` | `GetBool` | `GetBoolOption` |
| `GetDurationE` | `GetDuration` | `GetDurationOption` |
| `GetBytesE` | `GetBytes` | `GetBytesOption` |
| `GetPeriodE` | `GetPeriod` | `GetPeriodOption` |

`Get*E` は型付き `*ConfigError` を返します (missing / null / 型違い /
duration・byte・period の不正フォーマット)。未解決 placeholder 起因の失敗は
`errors.Is(err, hocon.ErrNotResolved)` で判定できます。panic 系列は同じ
`*ConfigError` を payload に panic し、`Get*Option` は `None` を返します。

### スライスゲッター

```go
cfg.GetStringSlice(path)   []string
cfg.GetInt64Slice(path)    []int64
cfg.GetIntSlice(path)      []int
cfg.GetConfigSlice(path)   []*Config
```

それぞれに `GetXxxSliceE` (error 返却) と `GetXxxSliceOption` 版あり。

### オブジェクトアクセス

```go
sub, err := cfg.GetConfigE("server")  // (*Config, error)、"server" スコープ
mustSub := cfg.GetConfig("server")    // panic 版
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
if v, ok := opt.Get(); ok {
    fmt.Println(v)
}
name := opt.OrElse("default")
```

### Unmarshal

```go
type ServerConfig struct {
    Host    string         `hocon:"host"`
    Port    int            `hocon:"port"`
    Timeout time.Duration  `hocon:"timeout,omitempty"`
    MaxBody hocon.ByteSize `hocon:"max-body,omitempty"`
    Tags    []string       `hocon:"tags"`
}

var s ServerConfig
err := cfg.Unmarshal(&s)

// map[string]any も対応
m := make(map[string]any)
err = cfg.Unmarshal(&m)

// UnmarshalPath はパス上の任意ノード (オブジェクト・配列・スカラー) を decode する:
var servers []ServerConfig
err = cfg.UnmarshalPath("servers", &servers)
```

`hocon` タグがないフィールドはフィールド名を小文字化したキーで検索し、`hocon:"-"` でフィールドをスキップする。キーがないとエラーになるが、`omitempty` のフィールド（既存値を保持）と、スライス・マップのフィールド（nil のまま）は例外。

マップのキー型は string 型 (`type Key string` のような名前付きの型を含む)、符号付き・符号なしの整数型、`bool` のいずれか。整数のキーはキー型のサイズの 10 進整数として厳密に読み (`"80"` は `map[uint16]` のキーになるが、`"1.0"` はならない)、`bool` のキーは bool の値と同じ表記 (`true`/`false`、`yes`/`no`、`on`/`off`) を受け付ける。読めないキーや、それ以外のキー型は、キーを名指しするエラーになる。

単位付きの値は getter と同じ規則で読む。`time.Duration` フィールドは `GetDuration` と同じ (`"30s"`、単位なしはミリ秒)、`hocon.ByteSize` フィールドは `GetBytes` と同じ (`"512MiB"`、単位なしはバイト、負の値はエラー)。値は `int64(s.MaxBody)` で取り出す。`null` はフィールドを変更しない (`Unmarshal` の他のスカラーと同じ。getter は `null` をエラーにする)。

`hocon.Period` フィールドは `GetPeriod` と同じ規則で読む: `"30d"`、`"2w"`、`"6mo"`、`"1y"`、または単位なしの整数 (日数)。週は `Days` に畳み込む。日付への適用は `t.AddDate(p.Years, p.Months, p.Days)`。`null` はフィールドを変更しない (`Unmarshal` の他のスカラーと同じ。getter は `null` をエラーにする)。

### パッケージ include (`include package(...)`)

`include package("<identifier>", "<file>")` は、Go パッケージが登録した HOCON を取り込みます。ライブラリは自分のモジュールの中にデフォルトを同梱し、アプリケーションはそのファイルがディスクのどこにあるかを知らずに取り込めます。これは実装横断の拡張（[E11](https://github.com/o3co/xx.hocon/blob/main/docs/extra-spec-conventions.md#e11)）で、Lightbend HOCON の一部ではありません。

提供側のパッケージは、ファイルを埋め込んで `init()` で登録します:

```go
package auth

import (
    _ "embed"

    "github.com/o3co/go.hocon"
)

//go:embed reference.conf
var reference []byte

func init() {
    if err := hocon.RegisterPackage("github.com/acme/auth", "reference.conf", reference); err != nil {
        panic(err)
    }
}
```

アプリケーションはそのパッケージを import して、パースより前に `init()` を走らせます。そのパッケージの他の機能を使わない場合は blank import にします:

```go
import _ "github.com/acme/auth"

cfg, err := hocon.ParseString(`include package("github.com/acme/auth", "reference.conf")`)
```

- identifier は慣例として提供側モジュールのパスです。file はスラッシュ区切りの相対パスで、`.` / `..` のセグメントは拒否されます（`*hocon.RegistrationError`）。
- バイト単位で同じ内容の再登録は何もしません。同じ identifier と file に違う内容を登録すると `*hocon.PackageCollisionError` になります（典型的には、2 つの import パスやメジャーバージョンが同じ identifier を登録したとき）。
- 誰も登録していないパッケージの include はエラーで、黙って空になることはありません。`include required(package(...))` は使えます。引数 1 つの `package("id")` はパースエラーです。
- テストでは `hocon.UnregisterPackage` / `hocon.ResetPackageRegistry` でグローバルレジストリを分離できます。

### エラー型

```go
var pe *hocon.ParseError   // 字句解析・構文解析エラー — Line, Col, FilePath を持つ
var re *hocon.ResolveError // 代入・include 解決エラー — Path を持つ
var ce *hocon.ConfigError  // Get*E の返却エラー / GetXxx パニックのペイロード — Path を持つ
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

# += 演算子（値を 1 要素として追記: a += b ≡ a = ${?a} [b]）
items = [1]
items += 2        // [1, 2]
items += [3, 4]   // [1, 2, [3, 4]]  （配列を渡すと 1 つのネスト要素として追記）

# インクルード
include "defaults.conf"
include file("overrides.conf")

# Duration・バイトサイズ
timeout   = "30s"
cache-ttl = "5m"
max-size  = "512MiB"
```

## パフォーマンス

Apple M4 Pro 上で `go test -bench`（Go 組み込みのベンチマーク）により計測。各イテレーションはパースと `GetString` 1 回を含みます。再現は `go test -bench=. -benchmem ./...`。

| シナリオ | ops/sec | 1 回あたり |
| --- | --- | --- |
| 小さい設定（10 キー） | ~173,000 | ~5.8 µs |
| 中くらいの設定（100 キー） | ~27,000 | ~38 µs |
| 大きい設定（1,000 キー） | ~2,000 | ~422 µs |
| 代入 10 個 | ~70,000 | ~14 µs |
| 代入 50 個 | ~16,000 | ~61 µs |
| 代入 100 個 | ~8,000 | ~121 µs |
| ネスト深さ 5 | ~168,000 | ~6.0 µs |
| ネスト深さ 10 | ~108,000 | ~9.3 µs |
| ネスト深さ 20 | ~64,000 | ~15.6 µs |

一般的なアプリケーション設定（起動時に 1 回読む）では、パースのコストは無視できます。1,000 キーの設定でも 1 ms 未満でパースできます。

## 比較

✅ 完全サポート / ⚠️ 部分的 / ❌ 非対応

### HOCON 実装

[gurkankaymak/hocon](https://github.com/gurkankaymak/hocon) **v1.4.0** と 2026-10-16 に比較。

| 機能 | go.hocon | gurkankaymak/hocon v1.4.0 |
| --- | :---: | :---: |
| 代入（`${path}`） | ✅ | ✅ |
| オプショナル代入（`${?path}`） | ✅ | ✅ |
| Include | ✅ | ✅ |
| `include required(...)` / `file(...)` | ✅ | ✅ |
| オブジェクト / 配列の連結 | ✅ | ✅ |
| 型変換 | ✅ | ✅ |
| Duration パース（`30s`、`5m`） | ✅ | ✅ |
| バイトサイズパース（`512MB`） | ✅ | ❌（文字列としてパース） |
| `+=` 追記 | ✅ | ✅ |
| 構造体 Unmarshal | ✅ | ❌ |
| panic しないアクセス | ✅（`GetXxxE` + `Option[T]`） | ✅（`GetXxxE`） |
| 環境変数フォールバック | ✅ | ✅ |
| 遅延解決（parse / withFallback / resolve） | ✅（v1.4.0 から） | ✅（v1.3.0 から） |
| `FromMap` / `Empty` | ✅（v1.4.0 から） | ⚠️（型付きの `Object{…}.ToConfig()`。`map[string]any` は不可） |

### 設定フレームワーク

[viper](https://github.com/spf13/viper) **v1.21.0**（現在も最新）と 2026-10-16 に比較。

| | go.hocon | viper v1.21.0 |
| --- | :---: | :---: |
| **フォーマット** | | |
| HOCON | ✅ | ❌ |
| JSON | ✅ | ✅ |
| YAML | ✅（`adapters/yaml`） | ✅ |
| TOML | ✅（`adapters/toml`） | ✅ |
| 環境変数 | ✅（フォールバック + `adapters/env`） | ✅ |
| .properties | ✅（include + `adapters/properties`） | ✅ |
| **機能** | | |
| 代入 | ✅ | ❌ |
| ファイル include | ✅ | ❌ |
| 型変換 | ✅ | ✅ |
| 構造体 Unmarshal | ✅ | ✅ |
| 監視 / リロード | ❌ | ✅ |
| リモート設定 | ❌ | ✅ |

## 仕様準拠

[Lightbend HOCON 仕様](https://github.com/lightbend/config/blob/main/HOCON.md) への準拠状況は [`docs/spec-compliance.md`](docs/spec-compliance.md) に項目単位で記載しており、以下の表はそこから計算されます — `TestDocs_ReadmeComplianceRates` が再計算し、表がずれるとビルドが fail します。実装横断のロールアップは [`xx.hocon/docs/compliance-matrix.md`](https://github.com/o3co/xx.hocon/blob/main/docs/compliance-matrix.md) を参照してください。

| 指標 | 状況 |
| --- | --- |
| 仕様全体（out-of-scope を含む） | **92.4%** |
| In-scope のみ | **100.0%** |
| Lightbend `equiv01`–`equiv05` + `test01`–`test13` | 合格（`TestLightbendEquiv` / `TestLightbendSuite`。`TestLightbendExpected` の `*-expected.json` の比較のうち 3 つは理由を明記して除外: `test01` は環境依存、`test02` と `test03` は Lightbend 1.4.6 との既知の差異） |
| [hocon2](https://github.com/o3co/hocon2) 準拠テスト（JSON/YAML/TOML/Properties 出力） | 合格 |

## フォーマットアダプタ

*他の*プログラムが所有する設定ファイルを HOCON としてマウントできます。自分のドキュメント内の `${...}` からその値を参照できます。

`adapters/` は **独立した Go モジュール** です（パーサー本体の依存ゼロを保つための構成）。そのため、インストールも独立しています:

```bash
go get github.com/o3co/go.hocon/adapters
```

```go
import (
    "github.com/o3co/go.hocon"
    "github.com/o3co/go.hocon/adapters/env"
)

// APP_DB__HOST=db.internal  ->  db.host
base, _ := env.Load(env.Options{Prefix: "APP_"})

// フォールバックを繋ぐまで、代入は未解決のままにしておく必要があります。
cfg, _ := hocon.ParseFileWithOptions("app.conf",
    hocon.DefaultParseOptions().WithResolveSubstitutions(false))

merged, _ := cfg.WithFallback(base).Resolve(hocon.ResolveOptions{})
```

```hocon
# app.conf — ${db.host} はマウントされた環境変数から解決される
url = "postgres://"${db.host}":"${db.port}
```

パーサーを import しても依存は一切増えません。依存が増えるのは、import したアダプタの分だけです。

| パッケージ | 備考 |
| --- | --- |
| `adapters/properties` | `java.util.Properties`。パーサー本体の `include` と構文層を共有 |
| `adapters/env` | prefix 付き名前空間の一括マウント。`.env` ファイルの読み込みにも対応 |
| `adapters/jsonc` | コメントと末尾カンマ付き JSON。依存なし |
| `adapters/json5` | JSON5。自前のスキャナで、依存なし |
| `adapters/toml` | `pelletier/go-toml/v2` を使用 |
| `adapters/yaml` | `goccy/go-yaml` を使用 |

### バージョニング

adapters モジュールは `adapters/vX.Y.Z` という独自のタグを持ちます。タグはビルド対象のコアのバージョンと対になり、最初に公開されたタグは `adapters/v1.11.0` です。`go.mod` が使用する API に対応するコアのバージョンを require しているので、`go get` を実行すれば必要なコアも一緒に入ります。

プレーンな JSON にアダプタは不要です — HOCON は JSON のスーパーセットなので、`hocon.ParseFile` がそのまま `.json` を受け付けます。環境変数を 1 つ読むだけの場合も不要で、それは `${?VAR}` の役目です。

外部データはあくまでデータです。マウントされた値の中の `${a.b}` は参照ではなくリテラルのテキストとして扱われます。そのファイルは、HOCON の構文に同意していない別のプログラムのものだからです。

### アダプタが厳格に拒否するもの

呼び出し側からは見えないもの — Go の map がたまたま辿った順序や、デコーダがたまたま読み止めた位置 — に意味が左右される入力は、取り込み時に拒否されます:

- **JSONC のドキュメントは値をちょうど 1 つだけ持つ。** その後ろに置けるのは空白とコメントだけで、それ以外は — `{"a":1} }` のような余分な閉じ括弧を含め — 黙って無視されずエラーになります。末尾のゴミを拒否するコストはトークン 1 個分で、その中身をデコードすることはありません。
- **JSONC のコメントはトークンを分割する。** コメントは空文字ではなく空白に置き換えられるため、`1/*x*/2` は数値 `12` ではなく構文エラーになります。`//` コメントは LF だけでなく CR でも終端します。
- **先頭の BOM は除去される。** 全フォーマット共通で、最初のキーに BOM が紛れ込むことはありません。
- **文字列形が一致する YAML のキーは衝突として扱う。** 文字列以外のスカラーキーはその文字列形になるため、`1.0:` と `"1":`、`~:` と `"null":` のような組は、どちらかの値が黙って消えるのではなく、両方の綴りと行番号を示すエラーになります。`<<:` のマージキーは対象外です — マージ元のキーを後から上書きするのは YAML 本来の意味論だからです。`yaml.FromValue` に渡したツリーでも同じ規則が適用されます。

YAML のスカラー解決はこのモジュールではなくライブラリの責務です — `010` が 8 なのか 10 なのかは `goccy` の答えであって、ここでの保証ではありません。`yaml.FromValue` はデコード済みのツリーを受け取るので、別のライブラリやスキーマが必要な呼び出し側は自分でデコードして、その結果を渡してください。詳細は [`adapters/README.md`](adapters/README.md) を参照。

## 関連プロジェクト

| プロジェクト | 言語 | レジストリ | 説明 |
|---------|----------|----------|-------------|
| [ts.hocon](https://github.com/o3co/ts.hocon) | TypeScript | [npm](https://www.npmjs.com/package/@o3co/ts.hocon) | TypeScript/Node.js 向け HOCON パーサー |
| [rs.hocon](https://github.com/o3co/rs.hocon) | Rust | [crates.io](https://crates.io/crates/hocon-parser) | Rust 向け HOCON パーサー |
| [py.hocon](https://github.com/o3co/py.hocon) | Python | [PyPI](https://pypi.org/project/hocon-parser/) | Python 向け HOCON パーサー |
| [hocon2](https://github.com/o3co/hocon2) | Go | [pkg.go.dev](https://pkg.go.dev/github.com/o3co/hocon2) | HOCON → JSON/YAML/TOML/Properties 変換 CLI |

4 つのパーサー実装（[ts.hocon](https://github.com/o3co/ts.hocon)、[rs.hocon](https://github.com/o3co/rs.hocon)、[go.hocon](https://github.com/o3co/go.hocon)、[py.hocon](https://github.com/o3co/py.hocon)）はすべて同じ Lightbend HOCON 仕様で追跡されています — 実装ごとの準拠率は [横断ロールアップ](https://github.com/o3co/xx.hocon/blob/main/docs/compliance-matrix.md) を参照してください。

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

## 既知の制限

- **`include url(...)`** は非対応です。リモート設定の取得はこのパーサーの対象外です。アプリケーションの HTTP クライアントで取得し、内容を `ParseString()` に渡してください。
- **`include classpath(...)`** は非対応です。JVM 固有の include 形式で、Java ランタイム以外に相当するものがありません。
- **監視 / リロードなし** — 設定はロード時にパースします。ライブリロードが必要なら、変更時に `ParseString()` / `ParseFile()` を呼び直してください。
- **ストリーミングパーサーなし** — 入力全体をメモリに読み込みます。

API の詳細は [pkg.go.dev](https://pkg.go.dev/github.com/o3co/go.hocon) を参照してください。

## セキュリティ上の注意

信頼できない HOCON 入力をパースするときは、次の点に注意してください:

- **include のパストラバーサル:** `include "../../../etc/passwd.conf"` は include 元ファイルのディレクトリ（`ParseString` では作業ディレクトリ）を基準に解決されます。信頼できない入力をパースするなら include パスを検証してください。
- **入力サイズ:** パーサーに入力サイズの上限はありません。信頼できない入力は、`ParseString()` を呼ぶ前にサイズを検証してください。

## ライセンス

Apache License 2.0 — [LICENSE](LICENSE) を参照。

Copyright 2026 1o1 Co. Ltd.
