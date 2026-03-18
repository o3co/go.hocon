# HOCON Go ライブラリ 設計仕様

**日付:** 2026-03-18
**著者:** o3co Inc.
**ライセンス:** Apache 2.0
**モジュール:** `github.com/o3co/go.lib`
**パッケージ:** `github.com/o3co/go.lib/hocon`
**参考:** https://github.com/gurkankaymak/hocon

---

## 概要

HOCON（Human-Optimized Config Object Notation）をパースするGoライブラリ。[Lightbend HOCON仕様](https://github.com/lightbend/config/blob/main/HOCON.md)への完全準拠を目標とする。`go.lib` モノレポの最初のパッケージとして設計。

**目標:**

- Lightbend HOCON仕様への完全準拠（スコープは下記参照）
- イディオマティックなGo API（Go 1.21+、generics使用）
- 必須値の取得失敗時はpanic（設定のfail-fast）
- 任意値には `Option[T]`
- structへの `Unmarshal`（encoding/json スタイル）
- Lightbend公式テストスイートの統合

**v1.0のスコープ外:**

- `include url("...")` および `include classpath("...")` 形式（ファイルベースのincludeのみ対応）
- `include required(...)` — includeファイルが存在しない場合は無条件でエラー
- キー内のsubstitution（`${?app.name}.host = ...`）
- `CONFIG_FORCE_x` JVMシステムプロパティオーバーライド規約
- マイクロ秒（`us`）duration単位（他のHOCON duration単位はすべて対応）
- TB/TiB超のバイトサイズ単位（PB, PiB, EB, EiB, ZB, ZiB, YB, YiB） — `int64`は約9 EiB超でオーバーフロー。実用的な設定では使用されない
- `ParseString` へのカスタムベースディレクトリ注入 — 文字列入力内のincludeディレクティブは `os.Getwd()` からの相対パスで解決。カスタムベースディレクトリが必要な場合は、設定を一時ファイルに書き出して `ParseFile` を使うこと

---

## アーキテクチャ

**パース → 解決** の二段階設計。パース段階で未解決ASTを構築し、解決段階でincludeの展開・substitutionの解決・オブジェクトのマージを行い、最終的なvalue treeを生成する。

```
入力文字列/ファイル
  → Lexer     → []Token
  → Parser    → AST（未解決ノードを含む）
  → Resolver  → Value tree（完全解決済み・イミュータブル）
  → Config    → 公開API
```

パースと解決は意図的に分離している。この分離により、resolverがsubstitutionのタイミング・自己参照substitution・循環参照検出・include展開を正確に扱える。シングルパス実装で頻発する問題を回避するための設計判断。

解決済みvalue treeは構築後**イミュータブル**。すべての `*Config` は同期なしに複数goroutineから並行して読み取り可能。

---

## パッケージ構成

```
github.com/o3co/go.lib/
└── hocon/
    ├── hocon.go          // エントリポイント: ParseString, ParseFile
    ├── config.go         // Config型, GetXxx, Has, Keys, WithFallback
    ├── value.go          // Value型: Object, Array, Scalar
    ├── option.go         // Option[T] ジェネリック型
    ├── unmarshal.go      // Unmarshal — structタグ対応
    ├── errors.go         // エラー型（行/列情報付き）
    ├── internal/
    │   ├── lexer/
    │   │   └── lexer.go      // トークナイザ
    │   ├── parser/
    │   │   ├── ast.go        // ASTノード定義
    │   │   └── parser.go     // AST構築
    │   └── resolver/
    │       └── resolver.go   // substitution解決、include処理
    └── testdata/
        ├── hocon/            // Lightbend公式テストスイート
        └── fixtures/         // 独自エッジケーステスト
```

`internal/` 以下はパッケージ外から参照不可。外部に公開するのは `hocon` パッケージのAPIのみ。

---

## パス構文

すべての `GetXxx(path string)` および `Has(path string)` メソッドは、ドット区切りのパス式を受け取る:

- `"server.host"` → `server` オブジェクト内の `host` キー
- `"arr.0"` → 配列 `arr` の最初の要素（整数セグメントで配列インデックス指定）
- パスは大文字小文字を区別する
- 空文字列 `""` は `ConfigError` でpanic
- 末尾ドット `"server."` は `ConfigError` でpanic
- パス式内のクォートセグメントは非対応（v1.0）

HOCONキーのドット記法（`a.b = 1`）はネストオブジェクト（`a: { b: 1 }`）のシンタックスシュガー。パス解決は生のキー文字列ではなく、解決済みのオブジェクトツリー上で行われる。

---

## 内部設計

### Lexer（`internal/lexer`）

HOCON入力をトークナイズし、エラー報告のために行・列番号を追跡する。

```go
type TokenType int

const (
    TokenInvalid TokenType = iota  // ゼロ値センチネル — 有効な入力からは生成されない
    TokenString
    TokenInt
    TokenFloat
    TokenBool
    TokenNull
    TokenLBrace, TokenRBrace
    TokenLBracket, TokenRBracket
    TokenComma, TokenColon, TokenEquals, TokenPlusEquals
    TokenSubstitution      // ${path}
    TokenOptSubstitution   // ${?path}
    TokenInclude
    TokenNewline, TokenEOF
)

type Token struct {
    Type  TokenType
    Value string
    Line  int
    Col   int
}
```

**数値トークンのルール:**

- 小数点や指数を含むトークン（`1.0`, `1e5`）→ `TokenFloat`。valueに生文字列を格納
- 純粋な整数リテラル → `TokenInt`。valueに生文字列を格納
- パーサが `TokenInt` → `int64`、`TokenFloat` → `float64` に変換する

**改行の扱い:**
各改行に対して `TokenNewline` を発行する。パーサは連結コンテキスト内で改行をvalue終端として使用する。現在の行が式の途中（`+=` の `+` の後や配列の内側）で終わる場合のみ次の行に値が続く。それ以外の場合、改行は現在のvalueを終端する。

**クォートなし文字列の終端文字:**
クォートなし文字列トークンは次のいずれかで終端する: `$`、`"`、`:`、`=`、`{`、`}`、`[`、`]`、`,`、`+`、`#`、`\`、`^`、`?`、`!`、`@`、`&`、ホワイトスペース、改行、またはEOF。Lightbend HOCON仕様のforbidden characters定義に従う。（`$` はsubstitutionの開始のため終端、`"` はクォート文字列の開始のため終端。）

**トリプルクォート文字列:**
`"""` デリミタ間のコンテンツはそのまま取得される — エスケープ処理は行われない（バックスラッシュはリテラル文字、`\n` は改行ではなく `\` と `n` の2文字）。各行の前後のホワイトスペースもそのまま保持される。終端は `"""` のみ。

### AST（`internal/parser/ast.go`）

未解決のパースツリーを表現する。SubstitutionとincludeはResolverが処理するためノードとして保持する。

```go
type Node interface{ node() }

type ObjectNode  struct{ Fields []FieldNode }
type FieldNode   struct{ Key []string; Value Node; Append bool } // Append = +=
type ArrayNode   struct{ Elements []Node }
type ScalarNode  struct{ Value any }           // string/int64/float64/bool/nil (null)
type ConcatNode  struct{ Nodes []Node }        // 文字列または配列の連結
type SubstNode   struct{ Path string; Optional bool } // ${path} / ${?path}
type IncludeNode struct{ Path string }         // ファイルベースのみ（v1.0）
```

**`ConcatNode` の型ディスパッチ:**
`ConcatNode` は文字列連結・配列連結にのみ生成される。オブジェクト連結は `ConcatNode` では表現されず、重複キーの再帰マージパスで処理される。Resolverは最初のsubstitution以外の解決済み要素を見て文字列 vs 配列モードを判定する:

- 最初の解決済み要素が `ArrayNode` → 配列連結。すべての要素が配列に解決されなければ `ResolveError`
- 最初の解決済み要素がスカラーまたは文字列 → 文字列連結。各要素をstring表現に変換
- 最初の解決済み要素が `ObjectNode` → `ResolveError`（オブジェクトは連結不可。重複キーマージを使うこと）

### Resolver（`internal/resolver`）

ASTを完全解決済みのvalue treeに変換する。

**Include展開:**

対応する構文形式:

- `include "file.conf"` — ベアクォートパス（Lightbend正規形式）
- `include file("file.conf")` — 明示的な `file()` 形式

両形式は同一の動作をする。サポートされていない形式（`url(...)`, `classpath(...)`）をパーサが検出した場合は即座に `ParseError` を返す — 解決を試みない。

`url(...)` および `classpath(...)` はv1.0のスコープ外。

解決の動作:

- `IncludeNode` → **includeするファイルのディレクトリ**を基準にファイルを読み込む（ルートではない）→ 再帰的にパース → 親の `ObjectNode` にマージ
- Resolverは現在のファイルパスのスタックを維持し、再帰的なinclude展開時の相対パス解決を正確に行う
- `ParseString` でincludeディレクティブを含む場合: ベースディレクトリはデフォルトで `os.Getwd()` になる。作業ディレクトリが取得できない場合は `ResolveError` を返す

**Substitution解決（`${path}` および `${?path}`）:**

- 現在の完全マージ済みvalue tree内で `path` をルックアップ（ドット区切りセグメント）
- 見つからない場合: **リテラル**パス文字列を使って `os.Getenv(path)` にフォールバック（例: `"server.host"` → `os.Getenv("server.host")`）。注意: ドットを含む環境変数名はshell非friendly。Lightbend仕様をリテラルに従ったもの。将来のバージョンでアッパーケース/アンダースコア変換を追加する可能性あり。
- それでも見つからない場合:
  - `${path}`（必須）: `ResolveError` を返す
  - `${?path}`（任意）: ノードを完全に削除。オブジェクトの場合はそのフィールドを、配列の場合はその要素を削除する

**自己参照substitution:**
フィールドは自身の前の値を参照して追記や前置ができる:

```hocon
path = ["/usr/bin"]
path = ${path} ["/usr/local/bin"]  # => ["/usr/bin", "/usr/local/bin"]
```

Resolverは自己参照の解決時にフォールバックconfig（直前のマージ状態）の `path` の値を使う。循環参照とは異なる。

**循環参照検出:**
Resolverは解決中のパスのスタックを維持する。パス `A` の解決中にパス `B` の解決が必要になり、さらにパス `A` が必要になった場合、`ResolveError` を返す。自己参照substitution（フォールバック値を使うもの）はこのチェックの対象外。

**`+=`（配列追記）:**
`FieldNode.Append = true` の意味: キーの現在値を（フォールバックまたは直前のマージから）ルックアップし、配列として扱い、新しい値を追記する。`key = ${?key} [newValue]` と等価。既存の値が配列でない場合は `ResolveError` を返す。

**重複キー:**

- 両方の値が `ObjectNode` に解決される場合: 再帰マージ（後のキーが優先）
- それ以外: 最後の値が勝つ

**`null` 値:**
HOCONの `null` はファーストクラスの値（`ScalarNode{Value: nil}`）。存在しないキーとは区別される:

- キーが明示的に `null` に設定されていても `cfg.Has("key")` は `true` を返す
- `null` 値に対する `cfg.GetString("key")` は `ConfigError` でpanicする — 理由: 型ミスマッチ（値は存在するが `null` であり文字列ではない）
- すべての `GetXxxOption` メソッド（スカラー、`GetConfigOption`、`GetConfigSliceOption`、スライスバリアント）は `null` 値に対して `None` を返す。ルールは統一: nullはどのOptionバリアントにも型付き表現を持たない

**ASTにおけるdurationとバイトサイズ:**
HOCONはdurationやバイトサイズ専用のトークン型を持たない。`10s` や `100KB` などの値は `ScalarNode` にプレーンな `string` として格納される。`GetDuration` と `GetBytes` はオンデマンドで文字列をパースする。`map[string]any` へのUnmarshal時、これらの値は `string` として現れる — 他のクォートなし文字列値と同様。

**`WithFallback` のセマンティクス:**
レシーバをフォールバックの上にディープマージした新しい `*Config` を返す。レシーバもフォールバックも変更しない。`fallback` が `nil` の場合は新しいインスタンスを生成せずにレシーバをそのまま返す。value treeはイミュータブル。`WithFallback` はポインタエイリアスではなくディープマージで新しいtreeを生成する。

### エラー型（`errors.go`）

```go
// ParseError はレキシングまたはパースに失敗した場合に返される。
type ParseError struct {
    Message  string
    Line     int
    Col      int
    FilePath string // includeファイル内の場合に非空
}

// ResolveError は解決に失敗した場合に返される（substitution、include、循環参照）。
type ResolveError struct {
    Message  string
    Path     string // HOCONのsubstitutionパス（例: "server.host"）
    Line     int    // substitutionが現れるソース行（不明の場合は0）
    Col      int    // ソース列
    FilePath string // includeの解決時のファイルパス
}

// ConfigError は GetXxx メソッドのpanicで使用される。
type ConfigError struct {
    Message string
    Path    string // HOCONアクセスパス（例: "server.host"）
}
```

`ParseString` と `ParseFile` はGoの標準 `error` にラップした `*ParseError` または `*ResolveError` を返す。詳細が必要な場合は型アサーションで取得できる。

---

## 公開API

### エントリポイント

```go
func ParseString(input string) (*Config, error)
func ParseFile(path string) (*Config, error)
```

`ParseFile` はI/Oエラー（ファイル未存在、権限なし）を `*ParseError`（`Line: 0, Col: 0`、`FilePath` にファイルパスを設定）としてラップする。解決中のincludeファイルのI/Oエラーも同様に、`FilePath` を設定した `*ResolveError` として返す。

### Config — スカラー値

以下の場合に `ConfigError` でpanic:

- パスが存在しない（configにキーが存在しない）
- 値が `null`（型ミスマッチ: nullは型付き値ではない）
- 値の型がリクエストされたGo型と一致しない（例: 整数値に対して `GetString` を呼ぶ）

型マッチングは厳格: HOCON整数は自動的にfloatに変換されず、文字列は数値にパースされない。

```go
// 文字列
func (c *Config) GetString(path string) string
func (c *Config) GetStringOption(path string) Option[string]

// 整数 — GetInt64が本体、GetIntはwrapper
func (c *Config) GetInt64(path string) int64
func (c *Config) GetInt64Option(path string) Option[int64]
func (c *Config) GetInt(path string) int                    // int(GetInt64(path))
func (c *Config) GetIntOption(path string) Option[int]      // wrapper

// 浮動小数点 — GetFloat64が本体、GetFloat32はナローイングwrapper。
// GetFloat32はfloat64→float32変換を行う: 範囲外の値は無音でを±Infになる
//（Goの標準float32変換の動作）。
func (c *Config) GetFloat64(path string) float64
func (c *Config) GetFloat64Option(path string) Option[float64]
func (c *Config) GetFloat32(path string) float32            // float32(GetFloat64(path))
func (c *Config) GetFloat32Option(path string) Option[float32] // wrapper

// ブール
func (c *Config) GetBool(path string) bool
func (c *Config) GetBoolOption(path string) Option[bool]

// HOCON duration。対応単位: ns/nanoseconds, ms/milliseconds, s/seconds,
// m/minutes, h/hours, d/days。"d"/"days"は 24*time.Hour にマップ。
// マイクロ秒（us）はv1.0のスコープ外。
func (c *Config) GetDuration(path string) time.Duration
func (c *Config) GetDurationOption(path string) Option[time.Duration]

// HOCONバイトサイズ（例: "100KB", "1MiB"）
func (c *Config) GetBytes(path string) int64
func (c *Config) GetBytesOption(path string) Option[int64]
```

### Config — スライス

```go
func (c *Config) GetStringSlice(path string) []string
func (c *Config) GetStringSliceOption(path string) Option[[]string]

func (c *Config) GetInt64Slice(path string) []int64
func (c *Config) GetInt64SliceOption(path string) Option[[]int64]
func (c *Config) GetIntSlice(path string) []int
func (c *Config) GetIntSliceOption(path string) Option[[]int]

// オブジェクトの配列 — 各要素を *Config に変換。
// 要素がオブジェクトでない場合は ConfigError でpanic。
func (c *Config) GetConfigSlice(path string) []*Config
func (c *Config) GetConfigSliceOption(path string) Option[[]*Config]
```

### Config — オブジェクト / 構造

```go
func (c *Config) GetConfig(path string) *Config
func (c *Config) GetConfigOption(path string) Option[*Config]

// Has は値がnullでもパスが存在すれば true を返す。
func (c *Config) Has(path string) bool

// Keys は現在のオブジェクトの直接の子キー名を単一パスセグメント
//（ドット区切りでない）で返す。{ a.b = 1 } の場合 Keys() は ["a"] を返す。
// GetConfig("a") で得た *Config に対する Keys() は ["b"] を返す。
// 順序: ソース内で重複するキーは最初の出現が位置を決める。
// WithFallback で生成されたconfigの場合: レシーバのキーが先（最初の出現順）、
// 次にフォールバック固有のキー（フォールバック内の最初の出現順）。
func (c *Config) Keys() []string
```

### Config — マージ / Unmarshal

```go
// WithFallback はレシーバをフォールバックの上にディープマージした新しいConfigを返す。
// レシーバもフォールバックも変更しない。
// fallbackがnilの場合はレシーバをそのまま返す。
func (c *Config) WithFallback(fallback *Config) *Config

// Unmarshal は `hocon` structタグを使ってconfigをvにマップする。
// vはstructへの非nilポインタであること。
func (c *Config) Unmarshal(v any) error
```

### Unmarshal structタグ

```go
type ServerConfig struct {
    Host    string      `hocon:"host"`
    Port    int         `hocon:"port"`
    TLS     *TLSConfig  `hocon:"tls,omitempty"`
    Aliases []string    `hocon:"aliases"`
    Workers []*Worker   `hocon:"workers"`
}

var s ServerConfig
err := cfg.Unmarshal(&s)
```

**タグのセマンティクス:**

- `hocon:"key"` — HOCONキーをこのフィールドにマップ。存在しない場合は `error` を返す（`GetXxx` がpanicするのとは異なり、`Unmarshal` はエラーを `error` 戻り値で返す — フィールド未存在でpanicしない）
- `hocon:"key,omitempty"` — HOCONパスが存在しない **または null** の場合、structフィールドは**変更しない**（事前にセットされた値を保持。ゼロ値で上書きしない）。`Has()` はnullに対してtrueを返すが、`omitempty` はnullを「値なし」として扱う
- タグなし → フィールド名を小文字にしたものをキーとして使用

**`Unmarshal` 時の型変換:**

- HOCON整数 → Go `int`, `int64`: 正確; → Go `float64`, `float32`: 拡大変換（float32は精度損失の可能性）
- HOCON float → Go `float64`: 正確; → Go `float32`: ナローイング（範囲外は±Infになる）
- HOCON文字列 → Go `string` のみ。数値型への変換なし
- HOCONブール → Go `bool` のみ
- HOCON配列 → Goスライス: 各要素を再帰的に変換。要素の型ミスマッチはerrorを返す
- HOCONオブジェクト → Go struct: structタグを使って再帰的に `Unmarshal`
- HOCONオブジェクトの配列 → Go `[]*StructType` または `[]StructType`: 各要素をstructとしてUnmarshal
- HOCONオブジェクト → `map[string]any`: 対応。値はGoネイティブ型（`string`, `int64`, `float64`, `bool`, `nil`, `[]any`, `map[string]any`）。durationとバイトサイズの値は**生文字列**として格納（例: `"10s"`, `"100KB"`）— `time.Duration` や `int64` には変換されない
- その他すべての型ミスマッチは `error` を返す（panicしない）

### Option[T]

```go
type Option[T any] struct{ /* 非公開 */ }

// コンストラクタ（内部使用。テストや合成のためにcallerからも利用可）
func Some[T any](v T) Option[T]
func None[T any]() Option[T]

func (o Option[T]) IsSome() bool
func (o Option[T]) IsNone() bool
func (o Option[T]) Get() (T, bool)
func (o Option[T]) OrElse(def T) T
```

`GetXxxOption` は存在しないパスと `null` 値に対して `None` を返す（nullは型付き表現を持たない）。

---

## HOCON仕様カバレッジ（v1.0）

| 機能 | ステータス | 備考 |
| --- | --- | --- |
| コメント（`#`, `//`） | 対応 | |
| `:` の同義語としての `=` | 対応 | |
| ルートブレースの省略 | 対応 | |
| カンマ省略 / 末尾カンマ | 対応 | |
| クォートなし文字列 | 対応 | Lightbend仕様のtermination chars準拠 |
| トリプルクォート文字列 | 対応 | |
| 重複キーマージ | 対応 | オブジェクトは再帰マージ、それ以外は後勝ち |
| 変数substitution `${path}` | 対応 | 未解決の場合はエラー |
| 任意substitution `${?path}` | 対応 | 未解決の場合はフィールドを削除 |
| 自己参照substitution | 対応 | フォールバック/直前の値を使用 |
| 環境変数フォールバック | 対応 | リテラルパス文字列で `os.Getenv(path)` |
| 配列追記 `+=` | 対応 | 既存値が配列でない場合はエラー |
| 文字列連結 | 対応 | |
| 配列連結 | 対応 | |
| オブジェクト連結 | 対応 | 重複キーの再帰マージで実現 |
| includeディレクティブ（ファイル） | 対応 | includeするファイルのディレクトリからの相対パス |
| include `url(...)` / `classpath(...)` | スコープ外 | v1.0はファイルベースのみ |
| `include required(...)` | スコープ外 | すべてのincludeはデフォルトで必須 |
| キー内のsubstitution | スコープ外 | |
| `null` 値 | 対応 | 存在しないキーと区別。`Has()` はtrueを返す |
| duration値 | 対応 | ns, ms, s, m, h, d（d = 24h）。usはスコープ外 |
| バイトサイズ値 | 対応 | B, KB, KiB, MB, MiB, GB, GiB, TB, TiB（PB+はスコープ外） |
| 循環参照検出 | 対応 | 解決時に ResolveError |
| `WithFallback` ディープマージ | 対応 | イミュータブル、新インスタンス |
| `CONFIG_FORCE_x` オーバーライド | スコープ外 | JVM規約 |

---

## テスト戦略

### Lightbend公式テストスイート

Lightbend configテストスイートを `testdata/hocon/` に配置。各 `.conf` ファイルに対応する `.json` の期待値ファイルが存在する。テストはディレクトリを自動的にスキャンする:

```go
func TestLightbendSuite(t *testing.T) {
    entries, _ := os.ReadDir("testdata/hocon")
    for _, conf := range confFiles(entries) {
        t.Run(conf, func(t *testing.T) {
            cfg, err := ParseFile("testdata/hocon/" + conf)
            // 対応する .json と比較
        })
    }
}
```

### レイヤー別カバレッジ

| レイヤー | スコープ |
| --- | --- |
| `internal/lexer` | トークン単位のユニットテスト |
| `internal/parser` | ASTノード単位のユニットテスト |
| `internal/resolver` | substitution・自己参照・include・循環参照・null・+=エラーのユニットテスト |
| `hocon`（公開API） | Lightbendスイート + Unmarshal / Option / GetBytes / null / GetConfigSlice の統合テスト |

---

## 設計の判断

**なぜ必須値にpanicを使うのか？**
設定値はシステムが起動するために必要。ゼロ値を暗黙的に返すことで設定ミスを隠蔽するのは危険。起動時にpanicすることは明示的でfail-fast — プログラマエラーに対するGoプログラムの扱いと一致する。

**なぜ二段階（AST + Resolver）なのか？**
HOCON仕様そのものが「パース → 解決」の二段階を前提に設計されている。シングルパス実装はsubstitutionのタイミング・自己参照値・includeの順序に苦労する。段階を分離することで各フェーズが独立してテスト可能になり、仕様通りの動作を保証できる。

**なぜ `WithFallback` をイミュータブルにするのか？**
構築後のvalue treeはイミュータブルであり、`*Config` は並行読み取りに安全。ミューテーションなしのマージにより同期が不要になり、データフローが明確になる。

**なぜGo 1.21+なのか？**
Generics（1.18+）により `Option[T]` を型別ボイラープレートなしに実装できる。Go 1.21は `slices`・`maps`・`cmp` 標準パッケージを追加しており、外部依存なしに実装を簡潔にできる。

**なぜ `ParseError` と `ResolveError` を分けるのか？**
パースエラー（構文エラー）と解決エラー（substitution未解決・循環参照）は異なるフェーズで異なるコンテキストを持つ。呼び出し側はこれを区別したい場合がある — 例えば構文エラーの行/列を表示したい場合と、変数未定義のsubstitutionパスを表示したい場合。

**なぜ `GetXxx` で `null` を型ミスマッチpanicにするのか？**
HOCONの `null` は「値の不在」ではなく明示的な値 — `Has()` がtrueを返す。しかし `GetString` などの型付きgetterはnullに対して意味のあるGo値を返せない。型ミスマッチの `ConfigError` でpanicすることは、整数値に `GetString` を呼ぶのと一貫性がある: キーは存在するが型が違う。`GetXxxOption` はnullに対して `None` を返し、呼び出し側にnull安全なパスを提供する。
