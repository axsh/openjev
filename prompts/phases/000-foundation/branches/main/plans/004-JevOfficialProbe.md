# 004-JevOfficialProbe

> **Source Specification**: `prompts/phases/000-foundation/branches/main/ideas/004-JevOfficialProbe.md`

## Goal Description

`features/jev-test` に、本家 Jev へ `features/decision-test/testdata/bank.json` を 1 回だけ送り、`wall_ms` と応答の形を確認する CLI を置く。API キーは gitignore 済みの `tmp/typesafe-api-key.txt` から読む。本家への通信は単体テストにも統合テストにも入れない。

## User Review Required

None.

## Requirement Traceability

| Requirement (from Spec) | Implementation Point (Section/File) |
| :--- | :--- |
| 1. モジュール `openjev/features/jev-test`、main は 1 つ、標準ライブラリのみ、decision-test を import しない、本家通信を `tests/` に入れない | `features/jev-test/go.mod`、`cmd/jev-test`、`tests/jev_probe_test.go`（httptest のみ） |
| 2. フラグ既定、1 リクエスト、分割も再試行もしない | `internal/cli/run.go` |
| 3. キーの Trim、Bearer、トークンを出さない、空キーは終了コード 2 | `internal/probe/probe.go`、`internal/cli/run.go` |
| 4. `state` / `questions` の検査、`model` 補完、ファイル非改変、30 件、出現順 | `internal/probe/probe.go` |
| 5. `wall_ms` は送信直前から本文読了まで。`elapsedMs` は別欄。ウォームアップ無し | `internal/probe/probe.go`、`internal/check/check.go` |
| 6. 形の確認。封筒 `{code,message,data}`。意味ラベルは置かない。非 2xx は内容確認しない | `internal/check/check.go`、`internal/cli/run.go` |
| 7. 人間可読の行順、`--json`、終了コード 0 / 1 / 2 | `internal/cli/run.go` |
| 検証シナリオ 3–8（リポジトリルートから既定パスで 1 回送る） | Verification Plan の公式 1 回実行。回帰の正本は httptest |
| 任意要件なし | 実装しない |

## Proposed Changes

ボトムアップ順に、テストファイルを各パッケージの実装より先に書く。

### logger

`features/decision-test/internal/logger` は import しない。同じ API の薄いラッパをこのモジュールに置く。呼び出し側は `log/slog` と `fmt.Print` をログ用途で使わない。stdout の計測レポートはログではなく、仕様の行形式を `io.Writer` へ書く。

#### [NEW] `features/jev-test/internal/logger/logger.go`

*   **Technical Design**:

```go
const levelTrace = slog.Level(-8)

type Logger struct{ s *slog.Logger }

func New(w io.Writer) *Logger
func (l *Logger) WithComponent(name string) *Logger
func (l *Logger) Info(msg string, kv ...any)
func (l *Logger) Warn(msg string, kv ...any)
func (l *Logger) Error(msg string, kv ...any)
func (l *Logger) Debug(msg string, kv ...any)
func (l *Logger) Trace(msg string, kv ...any)
```

*   **Logic**: `New` は `slog.NewTextHandler` の Level を `levelTrace` にする。`WithComponent` は `component` 属性を足す。`Trace` は `s.Log(context.Background(), levelTrace, msg, kv...)`。

### check

HTTP を知らない。質問定義と応答バイトから `content_ok` と理由と表示用の回答を決める。

#### [NEW] `features/jev-test/internal/check/check_test.go`

*   **Description**: 表駆動で形の合否を固定する。本家へは接続しない。
*   **Technical Design**:

```go
func TestCheckAcceptsFixture(t *testing.T)
func TestCheckRejects(t *testing.T)
func TestCheckEnvelope(t *testing.T)
func TestCheckNoulIgnoresConfidence(t *testing.T)
func TestCheckProbabilitySumNotRequired(t *testing.T)
```

*   **Logic**:
    *   質問は 3 件を基本にする。`queue` choice criteria キー `account_access`, `billing`。`frustration` score レベル 3（上限 2）。`is_urgent` noul。
    *   `TestCheckAcceptsFixture`: choice は `account_access`、確率は両キーで 0.5、confidence 1。score は 2（上限ちょうど）、確率キー `"0"`,`"1"`,`"2"`、legend は `{}`。noul は 0 と 1 の両境界を別ケースで見る（0 は受理）。`model` は `"jev-1.13.0"`。`usage` は `input_tokens` 1、`output_tokens` 0。`elapsedMs` 1200。`ContentOK` true、`ServerElapsedMs` は 1200、回答順は質問順。
    *   `TestCheckRejects` の行（いずれも `ContentOK` false、理由文字列に質問 ID とフィールド名）:
        *   回答から `queue` が欠ける → `queue` と `missing`
        *   回答に `extra` がある → `extra`
        *   `queue.type` が `score` → `queue` と `type`
        *   `queue.choice` が `nope` → `queue` と `choice`
        *   `queue.probabilities` のキーが criteria と不一致 → `queue` と `probabilities`
        *   `queue.confidence` が 1.1、および -0.1 → `queue` と `confidence`
        *   `frustration.score` が -0.1 と 2.1 → `frustration` と `score`
        *   `frustration.legend` が配列 → `frustration` と `legend`
        *   `is_urgent.noul` が -0.1 と 1.1 → `is_urgent` と `noul`
        *   `model` が `""` → `model`
        *   `usage.input_tokens` が 1.5、`usage.output_tokens` が -1 → `usage` と該当フィールド名
        *   本文が配列 → `response`
    *   `TestCheckEnvelope`: トップは `{"code":0,"message":"ok","data":{...answers...}}`。`elapsedMs` はトップにだけ置く。`data` を結果として使い、`ServerElapsedMs` は 50。
    *   `TestCheckNoulIgnoresConfidence`: noul に `confidence: 5` があっても `ContentOK` true。
    *   `TestCheckProbabilitySumNotRequired`: choice の確率が両方 0.2 でも `ContentOK` true。
    *   `usage` キーが無い応答は `ContentOK` true かつ `Usage == nil`。このケースは `TestCheckAcceptsFixture` のサブテストに入れる。

#### [NEW] `features/jev-test/internal/check/check.go`

*   **Technical Design**:

```go
type Question struct {
    ID          string
    Type        string
    ChoiceKeys  []string
    ScoreLevels int
}

type Usage struct {
    InputTokens  int
    OutputTokens int
}

type Answer struct {
    ID         string
    Type       string
    Choice     string
    Score      float64
    Noul       float64
    Confidence float64
    HasScore   bool
    HasNoul    bool
    HasConf    bool
}

type Report struct {
    Model           string
    ServerElapsedMs *float64
    Usage           *Usage
    ContentOK       bool
    Reasons         []string
    Answers         []Answer
}

func Check(questions []Question, body []byte) Report
```

*   **Logic**:
    *   トップを `map[string]json.RawMessage` に読む。失敗、または JSON がオブジェクトでないときは `Reasons: ["response: not a JSON object"]` で戻る。
    *   `code`、`message`、`data` がすべてあり、`data` がオブジェクト、かつその中に `answers` があるとき、結果オブジェクトは `data`。そうでなければトップ。
    *   `elapsedMs` は結果オブジェクトにあればそれを、無くトップにあればトップを、`float64` として `ServerElapsedMs` にする。キーが存在するのに数でないときは理由 `elapsedMs`。
    *   `model` は結果の JSON 文字列。欠ける、文字列でない、空文字は理由 `model`。
    *   `answers` がオブジェクトでなければ理由 `answers`。
    *   質問 ID の集合と回答キーの集合を比較する。欠ける ID は `<id>: missing answer`。余剰キーは `<id>: extra answer`。
    *   質問順に回答を見る。`type` が質問の `Type` と違えば `<id>: field type`。
    *   choice: `choice` が `ChoiceKeys` に含まれる文字列。`probabilities` のキー集合が `ChoiceKeys` と一致。各値と `confidence` は 0 以上 1 以下の数。違反は `<id>: field choice`、`<id>: field probabilities`、`<id>: field confidence`。
    *   score: `score` は 0 以上 `ScoreLevels-1` 以下。`probabilities` のキーは `"0"` から `strconv.Itoa(ScoreLevels-1)` まで過不足なく。各値と `confidence` は 0 以上 1 以下。`legend` はオブジェクト（`null` と配列は不可、`{}` は可）。違反は `<id>: field score`、`<id>: field probabilities`、`<id>: field confidence`、`<id>: field legend`。
    *   noul: `noul` は 0 以上 1 以下。`confidence` は読まない。付いていても理由にしない。
    *   確率の合計は見ない。
    *   `usage` が無いときは `Usage` は nil。あるときはオブジェクトで、`input_tokens` と `output_tokens` がそれぞれ 0 以上かつ `math.Trunc` と一致する数。違うときは `usage.input_tokens` または `usage.output_tokens`。
    *   `ContentOK` は `len(Reasons)==0`。理由は途中で打ち切らない。
    *   `Answers` は質問の出現順。パースできた値だけ埋める。

### probe

キー、入力、1 回の POST、`wall_ms`。

#### [NEW] `features/jev-test/internal/probe/probe_test.go`

*   **Technical Design**:

```go
func TestLoadBankInjectsModel(t *testing.T)
func TestLoadRejects(t *testing.T)
func TestReadKey(t *testing.T)
func TestPostOnceMeasuresWall(t *testing.T)
func TestPostKeepsTokenOutOfError(t *testing.T)
```

*   **Logic**:
    *   `TestLoadBankInjectsModel`: `../../../decision-test/testdata/bank.json` を読む（パッケージディレクトリからの相対。`go test` のカレントはパッケージディレクトリ）。送信バイトの `questions` は 30、`model` は `typesafe/jev-1.13`。読み込み後にファイルを読み直し、`model` キーが無いこと、バイト列が呼び出し前と一致すること。質問スライス長 30。先頭 ID は `queue`、型は `choice`。`ChoiceKeys` は空でない。
    *   `TestLoadRejects`: テーブル。`{}` は `state`。`{"state":"x"}` は `questions`。`{"state":"x","questions":{}}` は `questions`。`{"state":"","questions":{"q":{"type":"noul","instructions":"y"}}}` は `state`。壊れた JSON はエラー。いずれもエラーで、ファイルを書く関数は無い。
    *   `TestReadKey`: ファイル内容が `"  tok en \n"` のときトークンは `"tok en"`（両端の空白だけ除き、中の空白は残す）。空ファイルはエラー。
    *   `TestPostOnceMeasuresWall`: httptest が 50ms 待ってから `{"ok":true}` を 200 で返す。呼び出し回数 1。`Authorization` は `Bearer probe-token`。`Content-Type` は `application/json`。本文は送ったバイトと一致。`WallMs >= 50`。`Status == 200`。
    *   `TestPostKeepsTokenOutOfError`: ハンドラは 401 と本文 `denied`。`Post` はエラーにしない（HTTP 応答は結果）。トークン `sekret-token-value` は `Post` の戻りやログ用に組み立てるエラー文字列に含めない。このテストは `err == nil`、`Status == 401`、`string(Body) == "denied"` を見る。トークン非含有は cli テストが stderr 全体で見る。

#### [NEW] `features/jev-test/internal/probe/probe.go`

*   **Technical Design**:

```go
type Loaded struct {
    Body      []byte
    Questions []check.Question
}

type PostResult struct {
    Status int
    Body   []byte
    WallMs float64
}

func ReadKey(path string) (string, error)
func Load(path, defaultModel string) (Loaded, error)
func Post(ctx context.Context, url, token string, body []byte, timeout time.Duration, log *logger.Logger) (PostResult, error)
```

*   **Logic**:
    *   `ReadKey`: `os.ReadFile`。UTF-8 として `strings.TrimSpace`。結果が空なら `fmt.Errorf("api key is empty")`。ファイルが読めなければそのエラー。トークン値はエラーに埋めない。
    *   `Load`:
        1. ファイルを読む。`json.Unmarshal` で `map[string]json.RawMessage`。失敗は `fmt.Errorf("input json: %w", err)`。
        2. `state` が無い、`null`、`""`、`{}`、`[]` なら `errors.New("state is required")`。
        3. `questions` が無い、またはオブジェクトでない、または `dec.More()` が一度も真にならないなら `errors.New("questions is required")`。
        4. 質問ごとに `type` は `choice` / `score` / `noul`。それ以外は `fmt.Errorf("%s: type must be choice, score, or noul", id)`。choice の `criteria` はオブジェクトでキー 1 つ以上。score の `criteria` は配列で長さ 2 以上 10 以下でないときは `fmt.Errorf("%s: field criteria", id)`。noul の `criteria` は無くてよい。あるときはオブジェクト。
        5. 質問 ID と `ChoiceKeys` は `json.Decoder` のトークン順。`encoding/json` の map 順に依存しない。
        6. `model` が無い、`null`、または JSON 文字列が空なら、送信用マップに `defaultModel` を JSON 文字列で入れる。元のファイルには書き戻さない。`json.Marshal` したバイトが `Loaded.Body`。
    *   `Post`:
        1. `http.Client{Timeout: timeout}` を毎回作る。リトライ用の Transport は付けない。
        2. `start := time.Now()` の直後に `http.NewRequestWithContext` と `client.Do`。ヘッダは `Authorization: Bearer <token>` と `Content-Type: application/json` だけ。
        3. `Do` が失敗したときも `WallMs` は `time.Since(start)` のミリ秒。`Status` は 0。エラーを返す。エラー文字列に token を連結しない。
        4. 成功時は `io.ReadAll` の後に `WallMs` を取る。本文読了を含む。`resp.Body.Close()`。
        5. DEBUG `posting` に `url`、`body_size`、`timeout_ms`。DEBUG `post completed` に `status`、`duration_ms`、`body_size`。トークンと `Authorization` はログに出さない。TRACE は応答本文のみ（リクエストヘッダは出さない）。
        6. 呼び出し回数を増やすループは書かない。

### cli と main

#### [NEW] `features/jev-test/internal/cli/run_test.go`

*   **Technical Design**:

```go
func TestRunSuccessText(t *testing.T)
func TestRunSuccessJSON(t *testing.T)
func TestRunHTTPError(t *testing.T)
func TestRunEmptyKey(t *testing.T)
func TestRunBadInput(t *testing.T)
```

*   **Logic**:
    *   入力は temp に書く 1 質問 noul。`{"state":"hello","questions":{"is_urgent":{"type":"noul","instructions":"now?"}}}`。`model` は書かない。
    *   `TestRunSuccessText`: httptest が `{"model":"jev-1.13.0","answers":{"is_urgent":{"type":"noul","noul":0.95}},"elapsedMs":1200}` を返す。キーファイルは `"k\n"`。`Run` の引数は `--input` `--key-file` `--url` `--timeout 5s`。終了コード 0。stdout の行順は `status: 200`、`wall_ms:` で始まる行、`server_elapsed_ms: 1200`、`model: jev-1.13.0`、`questions: 1`、`content_ok: true`、その次が `is_urgent type=noul noul=0.95`。`usage_` 行は無い。httptest のヒット数は 1。送られた JSON の `model` は `typesafe/jev-1.13`。
    *   `TestRunSuccessJSON`: 同じ応答に `"usage":{"input_tokens":3,"output_tokens":1}` を足す。`--json`。stdout は 1 JSON。`wall_ms > 0`、`questions == 1`、`content_ok == true`、`status == 200`、`server_elapsed_ms == 1200`、`usage.input_tokens == 3`、`answers.is_urgent.noul == 0.95`。stdout にトークンは無い。
    *   `TestRunHTTPError`: 401 本文 `denied`。終了コード 1。stderr に `401` と `denied`。stdout と stderr にトークン `sekret-token-value` は無い。内容確認の回答行は stdout に無い。
    *   `TestRunEmptyKey`: キーファイルは空。終了コード 2。ヒット数 0。
    *   `TestRunBadInput`: 入力 `{}`。終了コード 2。ヒット数 0。

#### [NEW] `features/jev-test/internal/cli/run.go`

*   **Technical Design**:

```go
const (
    ExitOK    = 0
    ExitAPI   = 1
    ExitUsage = 2
)

type Options struct {
    Input    string
    KeyFile  string
    URL      string
    Model    string
    Timeout  time.Duration
    JSON     bool
}

func Run(args []string, stdout, stderr io.Writer, log *logger.Logger) int
```

*   **Logic**:
    *   `flag.NewFlagSet("jev-test", flag.ContinueOnError)`。出力先は `stderr`。既定は `--input features/decision-test/testdata/bank.json`、`--key-file tmp/typesafe-api-key.txt`、`--url https://api.typesafe.ai/v1/systemone`、`--model typesafe/jev-1.13`、`--timeout 10m`、`--json` false。パース失敗は 2。
    *   `ReadKey` または `Load` が失敗したら stderr にエラー 1 行、return 2。この時点では POST しない。
    *   DEBUG `probe starting` に `url`、`model`、`input`、`questions`、`key_file`（パスのみ）。
    *   `Post` を 1 回。`err != nil` なら ERROR `post failed`（`error`、`duration_ms`、`url`）。stderr にエラー。return 1。
    *   `status < 200 || status >= 300` なら ERROR `post rejected`（`status`、`duration_ms`、`body` は先頭 500 バイト）。stderr に `status: N`、`wall_ms: ...`、応答本文。内容確認は呼ばない。`--json` のとき stdout は `status`、`wall_ms`、`questions`（送った件数）、`content_ok: false` だけ。return 1。
    *   2xx なら `check.Check`。`content_ok` が false なら理由を 1 行ずつ stderr へ。return は 0 ではなく 1。
    *   人間可読 stdout の順は、仕様の次の行だけ。該当値が無い行は出さない。
        1. `status: <code>`
        2. `wall_ms: <FormatFloat f -1>`
        3. `server_elapsed_ms: <FormatFloat>`（`ServerElapsedMs != nil` のときだけ）
        4. `model: <model>`
        5. `questions: <len(questions)>`
        6. `content_ok: true` または `false`
        7. `usage_input_tokens: <n>` と `usage_output_tokens: <n>`（`Usage != nil` のときだけ、この順）
        8. 質問順に 1 行。choice は `<id> type=choice choice=<choice> confidence=<confidence>`。score は `<id> type=score score=<score> confidence=<confidence>`。noul は `<id> type=noul noul=<noul>`。
    *   `--json` の stdout は 1 オブジェクト。キーは `status`、`wall_ms`、`model`、`questions`、`content_ok`、`answers`。`server_elapsed_ms` と `usage` は nil でなければ含める。`answers` の値は choice が `type`,`choice`,`confidence`、score が `type`,`score`,`confidence`、noul が `type`,`noul`。`json.Marshal` の map 順は保証しない。
    *   INFO `probe finished` に `status`、`duration_ms`、`content_ok`、`questions`。
    *   トークンを stdout、stderr、ログの値に出さない。

#### [NEW] `features/jev-test/cmd/jev-test/main.go`

*   **Logic**: `logger.New(os.Stderr).WithComponent("jev-test")` を作り、`os.Exit(cli.Run(os.Args[1:], os.Stdout, os.Stderr, log))`。main はこれ以外を持たない。

#### [NEW] `features/jev-test/go.mod`

*   **Logic**: `module openjev/features/jev-test` と `go 1.24.0`。`require` は書かない。

### 統合テスト

本家 URL へは送らない。ビルド済みバイナリを httptest に向ける。仕様が禁止しているのは秘密と外部サービスであって、CLI の起動そのものではない。`scripts/process/integration_test.sh` にカテゴリは無いので `--specify` だけを使う。

#### [NEW] `tests/jev_probe_test.go`

*   **Technical Design**: ファイル先頭は `//go:build integration`。

```go
func TestJevProbe_BinaryPostsBankOnce(t *testing.T)
func TestJevProbe_BinaryRejectsUnauthorized(t *testing.T)
func TestJevProbe_BinaryUsage(t *testing.T)
```

*   **Logic**:
    *   バイナリは `filepath.Join(repoRoot, "bin", "jev-test.exe")`（`runtime.GOOS != "windows"` のときは拡張子なし）。無いときは `t.Fatal`。`t.Skip` は使わない。
    *   `cmd.Dir` はリポジトリルート。`--input` は付けず、既定の `features/decision-test/testdata/bank.json` を使わせる。`--key-file` は temp。中身は `probe-token`。実キーファイルは読ませない。
    *   `TestJevProbe_BinaryPostsBankOnce`: ハンドラは受けた `questions` から妥当な回答を作って 200 で返す（choice は最初のキー、確率はそのキーだけ 1 で残り 0、confidence 1。score は 0、確率 `"0"` が 1、legend `{}`。noul は 0.5）。`model` は `jev-1.13.0`、`elapsedMs` は 10、`usage` は両トークン 1。引数は `--url`、`--key-file`、`--json`、`--timeout 30s`。終了コード 0。ヒット数 1。`Authorization` は `Bearer probe-token`。受けた JSON の質問数 30、`model` は `typesafe/jev-1.13`。実行後の `bank.json` に `model` キーが無い。stdout の `wall_ms > 0`、`questions == 30`、`content_ok == true`、`status == 200`。
    *   `TestJevProbe_BinaryRejectsUnauthorized`: 401 本文 `denied`。終了コード 1。stdout と stderr に `probe-token` は無く、stderr に `401` と `denied` がある。
    *   `TestJevProbe_BinaryUsage`: キーファイルは空文字。終了コード 2。ヒット数 0。

### README

#### [MODIFY] `README.md`

*   **更新内容**: `## Official Jev probe` を `decision-test` の説明の後に足す。英語。`features/jev-test` が `bank.json` を `POST https://api.typesafe.ai/v1/systemone` に 1 回送り、`wall_ms` と回答の形を出すこと。キーは `tmp/typesafe-api-key.txt`（gitignore）で、値をコミットしないこと。実行例はリポジトリルートで `./bin/jev-test.exe`（Windows）。本家通信は `build.sh` にも統合テストにも入らないこと。

`prompts/specifications` は無い。源は `prompts/phases/000-foundation/branches/main/ideas/004-JevOfficialProbe.md` のままにする。

## Step-by-Step Implementation Guide

各ステップは「テストを書く → `./scripts/process/build.sh` がコンパイルまたはアサートで失敗することを確認 → 実装 → 同じスクリプトが通る → コミット」。

1. [ ] **check**
    *   Add `features/jev-test/go.mod`.
    *   Add `features/jev-test/internal/logger/logger.go`（check はログしないが、後段のパッケージが使う。このステップではテストしない）。
    *   Add `features/jev-test/internal/check/check_test.go`。実装が無い状態で `./scripts/process/build.sh` が失敗することを確認する。
    *   Add `features/jev-test/internal/check/check.go`。
    *   Run `./scripts/process/build.sh`. Commit `feat(jev-test): check official Jev answer shape`.
2. [ ] **probe**
    *   Add `features/jev-test/internal/probe/probe_test.go`。失敗を確認する。
    *   Add `features/jev-test/internal/probe/probe.go`。
    *   Run `./scripts/process/build.sh`. Commit `feat(jev-test): post one bank.json request and measure wall time`.
3. [ ] **cli と main**
    *   Add `features/jev-test/internal/cli/run_test.go`。失敗を確認する。
    *   Add `features/jev-test/internal/cli/run.go` と `features/jev-test/cmd/jev-test/main.go`。
    *   Run `./scripts/process/build.sh`. `bin/jev-test.exe`（Windows）が出来る。Commit `feat(jev-test): add CLI for one official Jev probe`.
4. [ ] **統合テストと README**
    *   Add `tests/jev_probe_test.go`. Edit `README.md`.
    *   Run `./scripts/process/build.sh`. Commit `test(jev-test): probe the built binary against a local server`.
5. [ ] **Verification Plan**（下記）。公式 1 回の `wall_ms` と `content_ok` を、キー値を書かずに本計画の末尾へ記録し、コミットする。

## Verification Plan

### Automated Verification

1.  **Build & Unit Tests**:

```bash
./scripts/process/build.sh
```

`features/jev-test` の単体テストが通り、`bin/jev-test.exe`（Windows 以外は `bin/jev-test`）が出来る。decision-test の単体テストもこのスクリプトが回す。失敗は無視しない。

2.  **Integration Tests**:

```bash
./scripts/process/build.sh && ./scripts/process/integration_test.sh --specify "TestJevProbe_"
```

*   **Log Verification**: テストログに `TestJevProbe_BinaryPostsBankOnce`、`TestJevProbe_BinaryRejectsUnauthorized`、`TestJevProbe_BinaryUsage` が PASS。llama-server は起動しない。`api.typesafe.ai` への接続ログは無い。

このスクリプトに `--categories` は無い。decision-test の統合テストは本計画の対象外なので、フィルタなしでは実行しない。

3.  **E2E Tests (新規/追加)**:

CLI として利用者に渡すため、統合テストは必須である。本家への通信は仕様が `tests/` から除外している。既存の `startServer` や llama ヘルパーは使わない。ローカル httptest が相手である。

#### [NEW] `tests/jev_probe_test.go`

*   **テストケース**: `TestJevProbe_BinaryPostsBankOnce`（30 質問、model 補完、終了コード 0、`content_ok`）、`TestJevProbe_BinaryRejectsUnauthorized`（終了コード 1、トークン非出力）、`TestJevProbe_BinaryUsage`（空キー、終了コード 2、HTTP 0 回）。
*   **検証ポイント**: ビルド済みバイナリがリポジトリルートの既定入力パスで 1 回だけ POST し、ファイルを書き換えず、2xx かつ形が合うときだけ終了コード 0 になる。

4.  **公式エンドポイントへの 1 回**（仕様の検証シナリオ 3–8）:

`./scripts/process/build.sh` の後、リポジトリルートで `./bin/jev-test.exe` を引数なしで 1 回実行する。続けて `./bin/jev-test.exe --json` を 1 回実行する。キーファイルが無い、または本家が 4xx/5xx を返したときは、質問を分割して送り直さない。状態と本文（キーは含めない）を計画末尾に残し、終了コード 1 のまま総合判定を条件付きにする。成功時は `wall_ms`、`questions == 30`、`content_ok`、終了コード 0 を記録する。この実行は回帰テストにしない。

### テスト項目設計のセルフレビュー（testing-rules §11）

ボトムアップ順: `check.Check`（末端の形） → `probe.Load` / `probe.Post`（ファイルと 1 回の HTTP） → `cli.Run`（終了コードと行） → 統合（ビルド済みバイナリ）。

| 観点 | 対応テスト |
| --- | --- |
| 正常系 | `TestCheckAcceptsFixture`、`TestRunSuccessText`、`TestRunSuccessJSON`、`TestJevProbe_BinaryPostsBankOnce` |
| 異常系・境界値 | `TestCheckRejects`（欠落、余剰、型、範囲の両端の外側、usage の小数と負）、`TestLoadRejects`、`TestRunEmptyKey`、`TestRunHTTPError`、score 上限ちょうどと noul の 0 |
| 外部連携の実動作 | `TestPostOnceMeasuresWall`（httptest）、統合のバイナリ 1 回。本家は仕様どおり回帰に入れず、検証シナリオの 1 回実行で見る |
| データの一貫性 | `TestLoadBankInjectsModel`（30 件、ファイルバイト不変）、統合が受けた質問数 30 と `model` |
| 状態遷移 | 終了コード 2（送る前）→ 1（非 2xx または `content_ok` false）→ 0（両方成功）。ウォームアップや再試行がヒット数 1 を超えない |
| 設定・構成の反映 | 既定 `model` が空の入力にだけ入る。`--json` が stdout を 1 オブジェクトにする。`--url` が httptest を向く |
| 副作用 | 入力ファイルのバイト不変。トークンが stdout / stderr / エラーに無い。空キーはヒット数 0 |

セルフレビュー結果: (1) 網羅性 — 形・1 回送信・終了コード・ファイル非改変が単体とバイナリの両方で揃えば、CLI として動作していると言える。本家の可用性は回帰の前提にしない。(2) 証拠の十分性 — 200 だけでなく質問数、`model`、`Authorization`、`wall_ms` の下限、行順を見る。(3) 迂回の排除 — 統合テストはライブラリではなく `bin/jev-test` を実行する。ヒット数 1 で再試行が無いことを見る。(4) 依存関係 — `check` が赤なら `cli` の成功は意味が無い。実装順は check → probe → cli → バイナリである。

### 総合判定（testing-rules §12）

全テスト完了後、次を確認し、本計画の末尾「総合判定結果」に記録する。

1. `t.Skip` と、ログの `SKIP` / 未実装のまま通した分岐。
2. テストログの `ERROR` / `panic`。401 テストの ERROR ログは期待どおりの拒否か。
3. 再試行や質問分割で成功していないか（ヒット数）。
4. `--url` が httptest であり、既定の本家 URL へテストが出ていないか。
5. `-count=1` で各テストが自分のサーバを持つか。
6. 新規パッケージ `check`、`probe`、`cli`、`cmd/jev-test` に対応するテストがあるか。
7. 公式 1 回実行時の本家の状態（HTTP 状態、`content_ok`）。失敗してもテストをスキップ扱いにしない。

## Documentation

`prompts/specifications` は無い。更新するのは `README.md` のみ。
