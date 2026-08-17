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
    log.Fatal(err) // フィールド欠落・型違いは起動時に fail する
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
- 他プログラムが所有する設定ファイルを読むフォーマットアダプタ — Properties、env、JSONC、TOML、YAML（下記参照）
- 外部依存ゼロ — 標準ライブラリのみ

## API

### パース

```go
hocon.ParseString(input string) (*Config, error)
hocon.ParseFile(path string)    (*Config, error)
```

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

`Get*E` は型付き `*ConfigError` を返します (missing / null / 型違い /
duration・byte の不正フォーマット)。未解決 placeholder 起因の失敗は
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

// UnmarshalPath はパス上の任意ノード (オブジェクト・配列・スカラー) を decode する:
var servers []ServerConfig
err = cfg.UnmarshalPath("servers", &servers)
```

`hocon` タグがないフィールドはフィールド名を小文字化したキーで検索する。`omitempty` はキーが存在しないとき、フィールドの既存値を保持する。

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

## 仕様準拠

[Lightbend HOCON 仕様](https://github.com/lightbend/config/blob/main/HOCON.md) への準拠状況は [`docs/spec-compliance.md`](docs/spec-compliance.md) に項目単位で記載しています。以下の表は 2026-05-13 時点のスナップショットです — 最新値は [`xx.hocon/docs/compliance-matrix.md`](https://github.com/o3co/xx.hocon/blob/main/docs/compliance-matrix.md) を参照してください。

| 指標 | 状況 |
| --- | --- |
| 仕様全体（out-of-scope を含む） | **71.8%** |
| In-scope のみ | **80.2%** |
| Lightbend `equiv01`–`equiv05` + `test01`–`test13` | 13/13 合格 |
| [hocon2](https://github.com/o3co/hocon2) 準拠テスト（JSON/YAML/TOML/Properties 出力） | 77/77 合格 |

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
| `adapters/toml` | `pelletier/go-toml/v2` を使用 |
| `adapters/yaml` | `goccy/go-yaml` を使用 |

### バージョニング

adapters モジュールは `adapters/vX.Y.Z` という独自のタグを持つ予定です。タグはビルド対象のコアのバージョンと対になり、最初のタグは `adapters/v1.10.x` になります。まだタグは打たれていないため、現時点の `go get` はデフォルトブランチの pseudo-version を解決します。いずれの場合も `go.mod` が使用する API に対応するコアのバージョンを require しているので、`go get` を実行すれば必要なコアも一緒に入ります。

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
