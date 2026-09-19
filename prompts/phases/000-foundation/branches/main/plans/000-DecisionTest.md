# 000-DecisionTest

> **Source Specification**: `prompts/phases/000-foundation/branches/main/ideas/000-DecisionTest.md`

## Goal Description

`features/decision-test` に、単一バイナリの CLI と Huma API を置く。既定の `direct` は MiniCPM5-2B Q4_K_M に対し、チャット補完を `max_tokens: 1` で 1 回だけ呼び、ラベル A…T の 1 トークン目 logprob をラベル間で softmax して Jev 形の `probabilities` を返す。`generation` は同じ入力から確率 JSON をトークン生成し、`both` は direct を正として両者を比べる。推論実体は CUDA 版 `llama-server` のサブプロセスである。

## User Review Required

計画に落とした、仕様が状態コードまで書いていない判断。実装前に否ならこの節だけ直す。

1. ウォームアップ未完了、またはラベルが単一トークンでないときはプロセスを `os.Exit` しない。ERROR ログを出し、`GET /health` は HTTP 503 と `ready: false` を返す。ログルールが Fatal / `os.Exit` を禁じているため。
2. `scripts/process/integration_test.sh` の `go test` に `-tags integration` を足す。現状はタグを渡さないので、`//go:build integration` のテストは実行されない。`--categories` はこのスクリプトに無く、今回は追加しない。
3. 説明が空文字 `""` の criteria は 422 にしない。選択肢本文は `<key>: `（コロンと空白のあとが空）にする。JSON `null` だけが「key のみ」である。
4. `method: generation` で生成検証が失敗したとき、`probabilities` に加えて `confidence` も付けない。分布が無いため。HTTP は 200 のまま。
5. `instructions` が空、`questions` が 0 件、`options.method` が未知のときも 422 にする。仕様が明示している空入力は `state` のみだが、プロンプトを組めないため同じ 422 に揃える。

## Requirement Traceability

| Requirement (from Spec) | Implementation Point (Section/File) |
| :--- | :--- |
| 1 単一バイナリ。引数なしまたは `serve` で起動、`decide` サブコマンド | Proposed Changes > cmd。`features/decision-test/cmd/decision-test/main.go` |
| 2 `POST /v1/systemone`、`GET /health`、422 / 502、Huma エラー形式 | Proposed Changes > api |
| 3 リクエスト形、`choice` のみ、criteria 2〜20、model 省略、method 既定 `direct` | Proposed Changes > domain |
| 4 レスポンス、`confidence = 1 - H(p)/ln(n)`、`both` は direct を正、`elapsedMs` | Proposed Changes > decision `service.go`、api |
| 5 read logits。`top_logprobs = max(64, 4*n)`、ラベル間 softmax、欠落は 502、grammar と logit_bias +100、`enable_thinking: false` | Proposed Changes > engine、decision `math.go` |
| 6 write tokens。stream、`max_tokens: 512`、`temperature: 0`、検証 6 規則、失敗は `valid: false` | Proposed Changes > engine `llamacpp.go`、decision `generate.go` |
| 7 プロンプト文言、選択肢行、質問 ID はモデルに渡さない、質問は順次 | Proposed Changes > prompt、decision `service.go` |
| 8 モデル URL、b11056 CUDA zip、起動引数、`/tokenize`、ミューテックス、Git に重みを入れない | Proposed Changes > scripts、engine、`.gitignore` |
| 9 CLI `decide`、testdata 2 件 | Proposed Changes > cli、testdata |
| 10 `settings/decision-test.yaml`、`internal/logger`、component `decision` | Proposed Changes > config、logger |
| 11 `Engine` は 3 メソッド。実装は llama-server HTTP のみ | Proposed Changes > engine `engine.go` |
| 検証シナリオ 1〜6 | Verification Plan > Integration / Unit |
| 任意要件（score、noul、255 択、応答 SSE、yzma、Ollama） | 実装しない。`Engine` をインターフェースのままにするだけ。理由は仕様の任意要件節と同じ |

## Proposed Changes

依存順。各パッケージでテストファイルを先に書く。単体テストはネットも llama-server も使わない。

### 設定と無視リスト

#### [NEW] `settings/decision-test.yaml`

*   **Description**: ポートとパスの唯一の定義。Go も setup スクリプトもこのファイルを読む。
*   **Technical Design**:

```yaml
api_host: 127.0.0.1
api_port: 8090
llama_url: http://127.0.0.1:18080
llama_host: 127.0.0.1
llama_port: 18080
llama_binary: third_party/llama.cpp/b11056/llama-server.exe
llama_build: b11056
llama_zip: llama-b11056-bin-win-cuda-12.4-x64.zip
cudart_zip: cudart-llama-bin-win-cuda-12.4-x64.zip
model_path: models/MiniCPM5-2B-Q4_K_M.gguf
model_id: minicpm5-2b-q4_k_m
model_url: https://huggingface.co/openbmb/MiniCPM5-2B-GGUF/resolve/2079a22f3beaa4e306449978533478fe0522f4b3/MiniCPM5-2B-Q4_K_M.gguf
log_path: ""
```

*   **Logic**: パスはプロセスの作業ディレクトリ相対。運用もテストもリポジトリルートを cwd にする。`log_path` が空なら標準エラー。空でないならそのファイルへ追記する。

#### [MODIFY] `.gitignore`

*   **Description**: 重み、llama.cpp 展開物、ビルド成果物を除外する。
*   **Logic**: 既存の `work/*` は残す。次を追加する。`bin/`、`models/`、`third_party/`。

### logger

#### [NEW] `features/decision-test/internal/logger/logger_test.go`

*   **Description**: コンポーネントとフィールドが出ること。
*   **テーブル**:
    *   `Info` が `component=decision` と `model_id=minicpm5-2b-q4_k_m` を含む 1 行をバッファに書く。
    *   `Debug` がレベル文字列 `DEBUG` を含む。
    *   `Error` が `error` フィールドを含む。

#### [NEW] `features/decision-test/internal/logger/logger.go`

*   **Description**: feature 内の唯一のログ出口。呼び出し側は `log`、`fmt.Print`、`slog` を直接使わない。
*   **Technical Design**:

```go
func New(w io.Writer) *Logger
func (l *Logger) WithComponent(name string) *Logger
func (l *Logger) Info(msg string, kv ...any)
func (l *Logger) Warn(msg string, kv ...any)
func (l *Logger) Error(msg string, kv ...any)
func (l *Logger) Debug(msg string, kv ...any)
func (l *Logger) Trace(msg string, kv ...any)
```

*   **Logic**: 実装の内側では `log/slog` の JSON ハンドラを使ってよい。外には出さない。キーはそのまま書く。本機能が使うキーは `component`、`model_id`、`question_id`、`method`、`path`、`status`、`duration_ms`、`error`、`body_prefix`。`body_prefix` は応答本文の先頭 500 バイト。`Error` には `error` と、分かる範囲の `path`、`status`、`body_prefix` を渡す。

### config

#### [NEW] `features/decision-test/internal/config/config_test.go`

*   **Description**: YAML の全フィールドがゼロ値のまま残らないこと。
*   **テーブル**:
    *   上記 YAML の例を一時ファイルに書き、`Load` の各フィールドが一致する。
    *   ファイルが無いときエラー。中身にパスは含まれてよいが、テストは `t.Fatal` で落とす（`t.Skip` 禁止）。

#### [NEW] `features/decision-test/internal/config/config.go`

*   **Technical Design**:

```go
type File struct {
    APIHost     string `yaml:"api_host"`
    APIPort     int    `yaml:"api_port"`
    LlamaURL    string `yaml:"llama_url"`
    LlamaHost   string `yaml:"llama_host"`
    LlamaPort   int    `yaml:"llama_port"`
    LlamaBinary string `yaml:"llama_binary"`
    LlamaBuild  string `yaml:"llama_build"`
    LlamaZip    string `yaml:"llama_zip"`
    CudaRTZip   string `yaml:"cudart_zip"`
    ModelPath   string `yaml:"model_path"`
    ModelID     string `yaml:"model_id"`
    ModelURL    string `yaml:"model_url"`
    LogPath     string `yaml:"log_path"`
}
func Load(path string) (File, error)
```

*   **Logic**: `gopkg.in/yaml.v3`。必須フィールド（`api_port`、`llama_url`、`model_id`、`model_path`、`llama_binary`）が空ならエラー。ホストとポートの既定値を Go の定数にしない。

### domain

#### [NEW] `features/decision-test/internal/domain/validate_test.go`

*   **Description**: 受理と 422 理由。推論は呼ばない。
*   **テーブル**（`Validate` の戻りエラー文字列を部分一致）:

| 名前 | 入力 | 期待 |
| :--- | :--- | :--- |
| ok_account | testdata と同じ queue / 3 criteria | nil。ラベル順は A=`account_access`、B=`billing`、C=`close` |
| state_empty | `state: ""` | エラーに `state` |
| state_blank | `state: "  "` | エラーに `state` |
| state_null | `state: null` | エラーに `state` |
| state_object | `state: {"ticket":1}` | nil。`PromptText()` は入力 JSON の空白を除いた生バイト（キー再整列しない） |
| criteria_one | criteria 1 個 | エラーに `2` と `20` |
| criteria_21 | criteria 21 個 | エラー |
| empty_key | `{"": "x", "b": "y"}` | エラーに `key` |
| duplicate_key | 生 JSON で同じ key を 2 回 | エラーに `duplicate` |
| null_description | `"billing": null` ともう 1 key | nil。billing の本文は `billing` |
| empty_description | `"billing": ""` ともう 1 key | nil。本文は `billing: ` |
| type_score | `type: score` | エラーに `not supported` |
| type_noul | `type: noul` | エラーに `not supported` |
| unknown_model | `model: other` | エラーに `model` |
| omit_model | `model` 無し | nil。解決後 ID は引数で渡した設定値 |
| method_default | `options` 無し | `MethodDirect` |
| method_bad | `method: stream` | エラーに `method` |
| no_questions | `questions: {}` | エラーに `questions` |
| empty_instructions | `instructions: ""` | エラーに `instructions` |
| question_order | 生 JSON で `z` の次に `a` | スライス順は z, a |

#### [NEW] `features/decision-test/internal/domain/types.go`

*   **Description**: リクエストと応答。`map` はキー順を捨てるので criteria と questions には使わない。
*   **Technical Design**:

```go
type Method string
const (
    MethodDirect     Method = "direct"
    MethodGeneration Method = "generation"
    MethodBoth       Method = "both"
)

type Request struct {
    Model     string
    State     State
    Questions []Question // JSON 出現順
    Method    Method
}

type State struct{ raw json.RawMessage }
func (s *State) UnmarshalJSON(b []byte) error
func (s State) PromptText() (string, error)

type Question struct {
    ID           string
    Type         string
    Instructions string
    Criteria     []Criterion // JSON 出現順
}
type Criterion struct {
    Key         string
    Description *string // nil は JSON null
    Text        string  // プロンプトに出す選択肢本文
}

type Response struct {
    Model     string            `json:"model"`
    Answers   map[string]Answer `json:"answers"`
    Usage     Usage             `json:"usage"`
    ElapsedMs int64             `json:"elapsedMs"`
}
type Answer struct {
    Type          string             `json:"type"`
    Choice        string             `json:"choice"`
    Probabilities map[string]float64 `json:"probabilities,omitempty"`
    Confidence    *float64           `json:"confidence,omitempty"`
    Method        Method             `json:"method"`
    Generation    *Generation        `json:"generation,omitempty"`
    Timings       *Timings           `json:"timings,omitempty"`
}
type Generation struct {
    Choice           string             `json:"choice"`
    Probabilities    map[string]float64 `json:"probabilities,omitempty"`
    Valid            bool               `json:"valid"`
    ValidationError  string             `json:"validation_error"`
    GeneratedText    string             `json:"generated_text"`
    TTFTMs           float64            `json:"ttft_ms"`
    TotalMs          float64            `json:"total_ms"`
    OutputTokens     int                `json:"output_tokens"`
}
type Timings struct {
    DirectMs     float64  `json:"direct_ms,omitempty"`
    GenerationMs float64  `json:"generation_ms,omitempty"`
    Ratio        *float64 `json:"ratio,omitempty"`
}
type Usage struct {
    InputTokens  int `json:"input_tokens"`
    OutputTokens int `json:"output_tokens"`
}
type Health struct {
    Ready          bool           `json:"ready"`
    Model          string         `json:"model"`
    LlamaReachable bool           `json:"llama_reachable"`
    LabelTokenIDs  map[string]int `json:"label_token_ids"`
    Error          string         `json:"error,omitempty"`
}
```

*   **Logic**:
    *   `Request` 自体は `UnmarshalJSON` を持つ。`questions` オブジェクトは `json.Decoder` の `Token` でキーを左から読む。標準の `map` に一度も入れない。`criteria` も同じ。
    *   重複キーは 2 回目を見た時点でエラー。
    *   `State.PromptText`: `null`、欠落、文字列の `TrimSpace` が空ならエラー。`"` で始まるなら `Unquote`。`{` または `[` で始まるなら `TrimSpace` した生 JSON を返す（`json.Marshal` し直さない）。
    *   `Criterion.Text`: `Description == nil` なら `Key`。否则 `Key + ": " + *Description`。
    *   ラベルは `string(rune('A' + i))`。i は criteria の添字。21 個目は Validate が先に拒むので T を超えない。

#### [NEW] `features/decision-test/internal/domain/validate.go`

```go
func Validate(raw []byte, configuredModelID string) (Request, error)
```

*   **Logic**: 上のテーブルの条件。`type` が `choice` 以外なら `fmt.Errorf("question type %q is not supported", typ)`。criteria 長が 2 未満または 20 超なら `fmt.Errorf("choice requires 2 to 20 options")`。model が空なら `configuredModelID`。空でなく設定値と違うなら `fmt.Errorf("unknown model %q", id)`。method 空なら `direct`。

### prompt

#### [NEW] `features/decision-test/internal/prompt/prompt_test.go`

*   **Description**: openjev の `messagesFor` とバイト単位で一致するゴールデン。質問 ID が文字列に含まれないこと。
*   **テーブル**:
    *   direct、account の 3 択。system と user が下の Logic の完成文字列と一致。
    *   generation、同じ入力。出力指示だけが generation 用。
    *   description が null の 2 択。行が `A. billing` と `B. other`（説明ありの方は `B. other: Else`）。
    *   state がオブジェクト。`State:` の次行が生 JSON。
    *   質問 ID `queue` が user 文に現れない。

#### [NEW] `features/decision-test/internal/prompt/prompt.go`

```go
type Message struct {
    Role    string `json:"role"`
    Content string `json:"content"`
}
func Messages(state string, instructions string, criteria []domain.Criterion, mode domain.Method) []Message
```

*   **Logic**: `mode` が `both` のときは呼ばない。呼び出し側が `direct` と `generation` を別々に渡す。

system 定数:

```text
Make the requested decision from the supplied state. Follow the output format exactly.
```

user は次を連結する。`optionText` は `Criterion.Text`。ラベルは A から。

```text
State:
{state}

Question:
{instructions}

Allowed options:
A. {optionText0}
B. {optionText1}
...

{outputInstruction}
```

direct の `outputInstruction`（ラベルは実際の列。区切りは `", "`）:

```text
Reply with exactly one option letter from: A, B, C.
```

generation の `outputInstruction` は次の文字列そのもの。改行位置も維持する。

```text
Estimate the probability that each allowed option is the correct decision.
Return only one JSON object mapping each option to its probability. Form every key as "<label>: <full option text>" using the allowed options above.
For example, if the unrelated options were "A. Route north" and "B. Route south", valid output would be:
{"A: Route north": 0.65, "B: Route south": 0.35}
For the actual decision, include every supplied option exactly once and in order. Each value must be a JSON number from 0 to 1, and the probabilities must sum to 1. Output JSON only, with no markdown or explanation.
```

account の Allowed options 完成形:

```text
A. account_access: Account access support
B. billing: Billing support
C. close: Close as resolved
```

generation が期待する JSON キーは、この行の `A. ` より後ろを使い `"A: account_access: Account access support"` とする。関数 `ExpectedGenerationKeys(criteria) []string` を同じパッケージに置き、`label + ": " + text` を返す。

### decision

#### [NEW] `features/decision-test/internal/decision/math_test.go`

*   **テーブル**:
    *   `Softmax([]float64{0, 0})` は各 `0.5`。
    *   `Softmax([]float64{1000, 0})` が Inf にならない。大きい側が 1 に近い。
    *   既知の logprob `ln(0.25), ln(0.25), ln(0.5)` を softmax すると合計 1 ± 1e-12。比は 1:1:2。
    *   `Confidence` が一様 3 要素で 0 ± 1e-12。
    *   `Confidence([]float64{1, 0, 0})` が 1。
    *   `Confidence([]float64{0.5, 0.5})` が 0。
    *   `PickLogprobs` が ID 一致で 3 つ取り、1 つ欠けるとエラー文に欠落ラベル `C` が入る。トークン文字列 `"A"` では照合しない。

#### [NEW] `features/decision-test/internal/decision/math.go`

```go
func Softmax(values []float64) []float64
func Confidence(probs []float64) float64
func ArgMax(probs []float64) int
func PickLogprobs(top []TokenLogprob, labels []Label) ([]float64, error)

type TokenLogprob struct {
    ID      int
    Logprob float64
}
type Label struct {
    Letter  string
    TokenID int
}
```

*   **Logic**:
    *   `Softmax`: `m = max(values)`、`exp(v-m)`、和で割る。全語彙の確率には戻さない。入力はラベル数と同じ長さの logprob。
    *   `Confidence`: `n <= 1` なら `1`。否则 `h = 0`。`p > 0` の項だけ `h -= p * math.Log(p)`。戻り値 `1 - h/math.Log(float64(n))`。
    *   `ArgMax`: 最大値の最初の添字。同率は criteria 順で先。
    *   `PickLogprobs`: 各ラベルの `TokenID` を `top` から探す。無ければ `fmt.Errorf("missing option logit for %s", letter)`。この型は `decision` パッケージに置く。`math.go` は `engine` を import しない。`service.go` が `engine.TokenLogprob` からこの型へ ID と Logprob をコピーしてから呼ぶ。こうすると math の単体テストを engine より先に書ける。

#### [NEW] `features/decision-test/internal/decision/generate_test.go`

*   **テーブル**（`ValidateGeneration`）:

| 名前 | テキスト | 期待 |
| :--- | :--- | :--- |
| ok | 期待キー 2 個が 0.65 と 0.35 | valid、choice は 0.65 側 |
| think | 先頭に `<think>secret</think>\n` がありその後が ok | valid。`GeneratedText` は除去後を保持してよいかは呼び出し側。検証は除去後を解析 |
| not_object | `[1,2]` | valid false、`expected one JSON object` |
| missing_key | キー 1 個 | `expected one probability for every exact option key` |
| extra_key | 期待に無いキー | 同じエラー |
| bad_number | `"no"` | `probabilities must be numbers from 0 to 1` |
| over_one | `1.1` と `-0.1` | 同じエラー |
| sum | `0.9` と `0.2`（和 1.1） | `probabilities must sum to 1` |
| sum_edge | 和が `1.02` | valid |
| sum_over_edge | 和が `1.021` | invalid |
| tie | `0.5` と `0.5` | choice は期待キーの先 |
| markdown | `` ```json `` で囲む | valid false（フェンスは剥がない） |

#### [NEW] `features/decision-test/internal/decision/generate.go`

```go
type GenResult struct {
    Valid           bool
    ValidationError string
    ChoiceKey       string
    Probabilities   map[string]float64
}
func ValidateGeneration(text string, criteria []domain.Criterion) GenResult
```

*   **Logic**: 順に実行し、失敗したらその時点の英語メッセージで `Valid: false` を返す。panic しない。
    1. `regexp.MustCompile(`(?i)^<think>[\s\S]*?</think>\s*`)` で先頭の think を 1 回だけ除く。`TrimSpace`。
    2. `json.Unmarshal` で `map[string]any`。失敗、またはトップがオブジェクトでない（配列・数値・文字列）なら `expected one JSON object`。実装は `json.NewDecoder` + `UseNumber` で `json.Delim('{')` を確認し、配列ならそのエラー。
    3. 期待キーは `prompt.ExpectedGenerationKeys`。`len` が違い、または期待キーが無ければ `expected one probability for every exact option key`。
    4. 各値は `float64` で有限、0 以上 1 以下。否则 `probabilities must be numbers from 0 to 1`。
    5. 和と 1 の絶対差が `0.02` より大きいなら `probabilities must sum to 1`。
    6. `ArgMax` の添字に対応する option key を `ChoiceKey` にする。JSON キーのラベルではなく criteria の key。
    *   解析に失敗し JSON でないときは `invalid JSON` ではなく、オブジェクト以外は規則 2 の文、`Unmarshal` 失敗は `invalid JSON`。openjev は `error.message` を使う。Go では規則 2 に落ちない構文エラーだけ `invalid JSON`。

#### [NEW] `features/decision-test/internal/decision/service_test.go`

*   **Description**: 偽物 `Engine` で方式の合成。HTTP も llama も使わない。
*   **テーブル**:
    *   `direct`: Engine の生成メソッドが呼ばれない。確率は softmax 結果。`Generation` は nil。`Timings.DirectMs` が 0 より大きい（テストは Engine が返す経過を使う）。`OutputTokens` は 1。
    *   `both`: トップの確率は direct。`generation.probabilities` は別物。`ratio == generationMs/directMs`。呼び出し順は Read の後に Generate。質問が 2 件なら質問順に Read。
    *   `generation` かつ valid: トップの choice は生成側。`OutputTokens` は生成側。
    *   `generation` かつ invalid: `Choice == ""`、`Probabilities` nil、`Confidence` nil、`Generation.Valid == false`、エラーは返さない。
    *   Read が欠落エラー: `service` は `ErrEngine` を返し、メッセージに欠落ラベルが残る。API 層が 502 にする。

#### [NEW] `features/decision-test/internal/decision/service.go`

```go
var ErrEngine = errors.New("engine")
type Service struct { Engine engine.Engine; Labels []engine.Label; Log *logger.Logger }
func (s *Service) Run(ctx context.Context, req domain.Request) (domain.Response, error)
```

*   **Logic**: 質問スライスを先頭から。各質問で `state.PromptText()` は 1 回。
    *   `direct` または `both`: `prompt.Messages(..., MethodDirect)` を `Engine.ReadLabelLogprobs` へ。`PickLogprobs` → `Softmax` → `ArgMax` → `Confidence`。ログ `Debug("direct completed", "question_id", id)`。
    *   `generation` または `both`: その後に `Debug("generation started", "question_id", id)`、`Messages(..., MethodGeneration)` を `Engine.Generate`。`ValidateGeneration`。
    *   `both` のトップレベルは direct の choice / probabilities / confidence。`Timings.Ratio` は `generation.TotalMs / directMs`。directMs が 0 なら ratio は付けず ERROR にはしない（テストでは 0 にしない）。
    *   `direct` の `Timings` は `DirectMs` のみ。`GenerationMs` と `Ratio` は omitempty で出ない。
    *   usage は全呼び出しの `PromptTokens` と `CompletionTokens` を加算。`ElapsedMs` は埋めない。API ハンドラが入口時刻から入れる。
    *   Engine エラーは `fmt.Errorf("%w: %s", ErrEngine, err)`。

### engine

#### [NEW] `features/decision-test/internal/engine/llamacpp_test.go`

*   **Description**: `httptest.Server` が llama-server のふりをする。
*   **検証するリクエスト JSON**（direct）:
    *   `max_tokens == 1`
    *   `temperature == 1`（0 が omit されていないこと。フィールドはポインタか、`json.Marshal` するマップで明示）
    *   `top_k == 0`、`top_p == 1`
    *   `logprobs == true`
    *   `top_logprobs == max(64, 4*len(labels))`。3 択なら 64。20 択なら 80。
    *   `cache_prompt == false`
    *   `chat_template_kwargs.enable_thinking == false`
    *   `grammar` が `root ::= "A" | "B"`（2 択のとき。スペース区切りの ` | `）
    *   `logit_bias` がオブジェクト `{"54": 100, "55": 100}`。配列形式にはしない。
*   **SSE**: 次を返し、`TTFT` が 0 より大きく、`Usage.CompletionTokens == 4`、本文が `{"A: x": 1}` になること。最初の空 delta は TTFT にしない。

```text
data: {"choices":[{"delta":{"content":""}}]}

data: {"choices":[{"delta":{"content":"{"}}]}

data: {"choices":[{"delta":{"content":"\"A: x\": 1}"}}],"usage":{"prompt_tokens":3,"completion_tokens":4}}

data: [DONE]
```

*   **tokenize**: `{"content":"A","with_pieces":true}` に対しトークンが 2 個なら `ResolveLabels` がエラー。1 個なら ID を採用する。
*   **直列**: 2 つの `ReadLabelLogprobs` を並行に呼び、サーバ側で重なりが無い（前の応答前に次のリクエストが来ない）。

#### [NEW] `features/decision-test/internal/engine/engine.go`

```go
type Label struct {
    Letter  string
    TokenID int
}
type TokenLogprob struct {
    ID      int
    Logprob float64
}
type ReadResult struct {
    Top              []TokenLogprob
    PromptTokens     int
    CompletionTokens int
    ElapsedMs        float64
}
type GenRaw struct {
    Text             string
    TTFTMs           float64
    TotalMs          float64
    PromptTokens     int
    CompletionTokens int
}
type Engine interface {
    ReadLabelLogprobs(ctx context.Context, messages []prompt.Message, labels []Label) (ReadResult, error)
    Generate(ctx context.Context, messages []prompt.Message) (GenRaw, error)
    Health(ctx context.Context) (reachable bool, err error)
}
```

*   **Logic**: メソッドは 3 個。yzma 実装は置かない。`engine` は `decision` を import しない。`decision` も `engine.TokenLogprob` を `PickLogprobs` の引数には使わず、`service.go` でフィールドをコピーする。import 循環と、math テストが engine に依存することを避ける。

#### [NEW] `features/decision-test/internal/engine/llamacpp.go`

```go
type Client struct { baseURL string; http *http.Client; mu sync.Mutex; log *logger.Logger }
func NewClient(baseURL string, log *logger.Logger) *Client
func ResolveLabels(ctx context.Context, c *Client) ([]Label, error)
func (c *Client) Warmup(ctx context.Context) error
```

*   **Logic**:
    *   `mu` は `ReadLabelLogprobs` と `Generate` と `Warmup` の全体を囲む。`Health` は `GET {base}/health` だけでロックしない。
    *   direct のボディは次のキーを必ず含む。数値 0 と `false` が落ちないよう `omitempty` を付けない。

```json
{
  "messages": [],
  "max_tokens": 1,
  "temperature": 1,
  "top_k": 0,
  "top_p": 1,
  "logprobs": true,
  "top_logprobs": 64,
  "logit_bias": {"54": 100},
  "grammar": "root ::= \"A\" | \"B\"",
  "cache_prompt": false,
  "chat_template_kwargs": {"enable_thinking": false}
}
```

    *   `top_logprobs` は `n := len(labels); v := 64; if 4*n > v { v = 4*n }`。
    *   応答は `choices[0].logprobs.content[0].top_logprobs[]` の `id` と `logprob`。`id` が無い要素は無視する。
    *   generation のボディ: `stream: true`、`max_tokens: 512`、`temperature: 0`、`cache_prompt: false`、`chat_template_kwargs.enable_thinking: false`。`logprobs` は送らない。
    *   検証で MiniCPM5 は、説明が 1 語の選択肢（`legitimate: Legitimate`）のキーを `A: legitimate` に省略した。仕様のキー完全一致を満たすため、user メッセージの `A. ...` 行から GBNF を作り、`grammar` でキー文字列と 0〜1 の数値だけを許す。確率の値はモデルが書く。account のように説明が長い場合も同じ文法を付ける。
    *   SSE は `bufio.Scanner`。`data: ` 接頭辞。`[DONE]` で終了。`delta.content` が非空の最初の時刻が `TTFTMs`。`usage` は最後に付いたものを使う。
    *   非 2xx はエラー。本文先頭 500 バイトをログの `body_prefix` に載せる。
    *   `ResolveLabels`: 文字 `A` から `T` まで 20 回 `POST /tokenize`。ボディ `{"content":"A","with_pieces":true}`。`tokens` が長さ 1 でなければ `fmt.Errorf("label %s is not a single token", letter)`。
    *   `Warmup`: messages は `[{role:user, content:"Reply with the single word ready."}]`、`max_tokens: 1`、`temperature: 0`、`cache_prompt: false`、`enable_thinking: false`。`choices` が空ならエラー。
    *   HTTP クライアントのタイムアウトは 10 分。direct は通常短いが、初回のコンパイル待ちを同じクライアントで行う。

### api

#### [NEW] `features/decision-test/internal/api/api_test.go`

*   **Description**: `humatest` または `httptest` で Huma ルータを起こす。Engine は偽物。
*   **ケース**:
    *   account ボディ `method: direct` が 200。`answers.queue.choice` が偽物 softmax の argmax。`probabilities` の和が 1 ± 1e-6。`generation` キーが JSON に無い。`confidence` が 0〜1。OpenAPI のそのフィールド説明に `1 - H(p) / ln(n)` が含まれる（`api.OpenAPI()` の YAML を文字列検索）。
    *   criteria 1 個、21 個、`type: score`、`state: ""` がそれぞれ 422。偽物 Engine の呼び出し回数が 0。
    *   偽物が欠落エラーを返すと 502。本文は Huma の `detail` を含み、欠落ラベルが残る。
    *   `GET /health` は準備完了なら 200、`ready: false` を渡したとき 503。

#### [NEW] `features/decision-test/internal/api/api.go`

```go
func Register(api huma.API, svc *decision.Service, health func(context.Context) domain.Health)
```

*   **Logic**:
    *   `POST /v1/systemone`。operation ID `post-systemone`。入力ボディは `json.RawMessage` とし、`domain.Validate` に渡す。Validate エラーは `huma.Error422UnprocessableEntity`。
    *   `errors.Is(err, decision.ErrEngine)` は `huma.Error502BadGateway`。detail はエンジンのメッセージ。
    *   `elapsedMs` はハンドラ入口の `time.Now()` から、Validate と `Service.Run` の両方を終えた時点までのミリ秒。`Service.Run` は `ElapsedMs` を埋めない。仕様の「検証を含む往復」に合わせ、Validate の時間を落とさない。
    *   `confidence` の schema doc は `1 - H(p) / ln(n). H is the natural-log entropy of the option distribution. 1 when peaked, 0 when uniform. Not Jev's unpublished definition.`
    *   `GET /health`。`ready == false` ならステータス 503。Huma は成功型とエラー型を分ける。未準備は `huma.NewError(503, health.Error)` にせず、ボディを返したいので出力構造の `Status` を 503 にできる Huma の `Status` フィールドを使う。準備完了は 200。`LabelTokenIDs` は A…T の 20 個。

### cli

#### [NEW] `features/decision-test/internal/cli/decide_test.go`

*   **Description**: HTTP サーバを `httptest` で立て、`decide` がその URL へ POST する。
*   **ケース**:
    *   `--json`: 標準出力がサーバのボディと一致。終了コード 0。
    *   `--method both` がボディの `options.method` を上書きする（入力 JSON は `direct`、送られた JSON は `both`）。
    *   `--json` 無し: 出力に `account_access`、`billing`、`close`、各確率の小数、`direct_ms`、`generation_ms`、`ratio` が含まれる。終了コード 0。
    *   サーバ 422: 終了コード非 0。標準エラーに status が入る。

#### [NEW] `features/decision-test/internal/cli/decide.go`

```go
type Options struct {
    Input  string
    Server string
    Method string
    JSON   bool
}
func Decide(ctx context.Context, opt Options, stdout, stderr io.Writer) error
```

*   **Logic**: 入力ファイルを読み、`method` フラグが空でなければ JSON の `options.method` を上書きする。`options` オブジェクトが無ければ作る。`POST {server}/v1/systemone`。2xx 以外は `fmt.Errorf("status %d", code)`。
    *   `--json` ならボディをそのまま stdout。
    *   否则、質問 ID ごとに次の行を出す。確率は `fmt.Sprintf("%.3f", p)`。ratio が無い方式ではその行を出さない。

```text
queue
  choice: account_access
  confidence: 0.710
  account_access  0.910
  billing  0.030
  close  0.060
  direct_ms: 95.200
  generation_ms: 2310.700
  ratio: 24.270
```

### cmd と testdata

#### [NEW] `features/decision-test/testdata/account.json`

*   **Logic**: `method` は付けない（既定 direct をテストが上書きする）。state のアポストロフィは仕様のまま Unicode `‘` `’`。

```json
{
  "state": "A customer says a password reset succeeded, but every login attempt still returns ‘account locked’. Two unlock emails were requested and neither arrived.",
  "questions": {
    "queue": {
      "type": "choice",
      "instructions": "Which queue should handle this request?",
      "criteria": {
        "account_access": "Account access support",
        "billing": "Billing support",
        "close": "Close as resolved"
      }
    }
  }
}
```

#### [NEW] `features/decision-test/testdata/email.json`

```json
{
  "state": "An email claims to be from the payroll team and says the recipient’s salary payment will be suspended today. It comes from payroll-review@outlook.com and links to a non-company sign-in page asking for a password and verification code.",
  "questions": {
    "classification": {
      "type": "choice",
      "instructions": "How should this email be classified?",
      "criteria": {
        "legitimate": "Legitimate",
        "spam": "Spam",
        "phishing": "Phishing"
      }
    }
  }
}
```

#### [NEW] `features/decision-test/go.mod`

*   **Logic**: `module openjev/features/decision-test`、`go 1.24`。依存は `github.com/danielgtaylor/huma/v2`、`github.com/spf13/cobra`（humacli が使う）、`gopkg.in/yaml.v3`。

#### [NEW] `features/decision-test/cmd/decision-test/main.go`

*   **Description**: 唯一の `main`。`go build -o bin/decision-test ./...` が通るのはこのため。
*   **Logic**:
    *   `humacli.New` の `OnStart` で `start`。フラグ `--config` の既定は `settings/decision-test.yaml`。`--port` が 0 でなければ YAML の `api_port` を上書きする。この既定パスだけは、利用者がフラグを省略するための CLI の help 既定であり、待受アドレスそのものは YAML から読む。
    *   `start`: config を読む。`log_path` が空でなければそのファイルを logger の出力にする。`engine.NewClient` → `ResolveLabels`。失敗なら ERROR ログを出し、`ready=false` の health だけを持つサーバを listen する（`os.Exit` しない）。成功なら `Warmup`。失敗時も同様に 503。成功なら `decision.Service` にラベルを渡し listen。INFO で `api_port` と `model_id` と `llama_url` を出す。
    *   cobra の `serve` は `OnStart` と同じ `start` を呼ぶ。引数なしは humacli の既定で `start`。
    *   `decide` はサーバを起動しない。フラグ `--input`（必須）、`--server`（空なら YAML の `api_host` と `api_port` から `http://{host}:{port}`）、`--method`、`--json`、`--config`。
    *   listen アドレスは `fmt.Sprintf("%s:%d", cfg.APIHost, cfg.APIPort)`。

### setup スクリプト

各スクリプトは `--help` を持ち、`set -euo pipefail`。作業ディレクトリはリポジトリルート想定。YAML 読みは `awk` で `^key:` の値を取る関数を 3 本で複製してよい（共通ライブラリは作らない）。

#### [NEW] `scripts/setup/install_llama_cpp.sh`

*   **Logic**: `llama_build`、`llama_zip`、`cudart_zip`、`llama_binary` のディレクトリを YAML から読む。`gh release download {build} --repo ggml-org/llama.cpp --pattern {zip} --pattern {cudart}` で `third_party/llama.cpp/{build}/` に展開する。両 zip を同じディレクトリへ unzip し、`llama-server.exe` とその横の CUDA DLL が揃うようにする。既にバイナリがあれば何もしない。

#### [NEW] `scripts/setup/download_model.sh`

*   **Logic**: `model_url` と `model_path`。`curl -L --fail -o` で取得。既存ファイルがあれば何もしない。SHA は仕様に無いので検証しない。サイズが 0 なら失敗。

#### [NEW] `scripts/setup/run_llama_server.sh`

*   **Logic**: 次の引数で exec する。値は YAML。思考無効化の `--reasoning off` は付けない。

```text
{llama_binary} -m {model_path} -c 2048 -b 512 -ngl 99 -np 1 --jinja --host {llama_host} --port {llama_port}
```

### 統合テストランナー

#### [MODIFY] `scripts/process/integration_test.sh`

*   **Logic**: `go_test_args` を `("-v" "-count=1" "-tags" "integration")` にする。`--specify` は従来どおり `-run` に追加。`--categories` は足さない。help 文言に `-tags integration` を 1 行追記する。

### 統合テスト（E2E 相当）

GUI は無い。VS Code 用 E2E ヘルパーもリポジトリに無い。実 llama-server と実バイナリを叩く `tests/` が、この機能の E2E である。手動の curl 確認は計画しない。

#### [NEW] `tests/go.mod`

*   **Logic**: `module openjev/tests`、`go 1.24`。feature モジュールは import しない。標準ライブラリだけ。

#### [NEW] `tests/decision_systemone_test.go`

*   **テストケース**: ファイル先頭 `//go:build integration`。
    *   `TestDecisionSystemOne_Health`: `settings/decision-test.yaml` の `llama_url` へ `GET /health`。失敗なら `t.Fatal`（skip しない）。`../bin/decision-test.exe`（`runtime.GOOS != "windows"` なら拡張子なし）を、cwd がリポジトリルートになるように起動する。`GET /health` が 200 かつ `ready==true`、`model==minicpm5-2b-q4_k_m`、`label_token_ids` に A から T の 20 キーがあり各値は正の int。テスト終了でプロセスを止める。
    *   `TestDecisionSystemOne_DirectAccount`: 上記サーバに account.json を `method: direct` で POST。仕様シナリオ 2 の 2〜6。`usage.output_tokens==1`。`generation` が無い。
    *   `TestDecisionSystemOne_GenerationEmail`: email.json を `method: generation`。`generation.valid==true`。`generated_text` を JSON 解析しキーが `A: legitimate: Legitimate`、`B: spam: Spam`、`C: phishing: Phishing`。和が 1 ± 0.02。トップの `choice` が argmax の key。`ttft_ms` と `total_ms` が 0 より大きく `ttft_ms <= total_ms`。
    *   `TestDecisionSystemOne_BothOrder`: account を `both`。トップ確率と `generation.probabilities` が両方ある。`ratio` が `generation_ms/direct_ms` と相対誤差 1e-6。`log_path` を一時ファイルにした設定コピーで起動し、ログに `direct completed` のあとに `generation started` がある。`question_id` は `queue`。
*   **検証ポイント**: 確率の和、choice が criteria の key、生成 JSON のキー完全一致、ログ順序。正解ラベルが `account_access` であることは仕様が要求していないので断言しない。

## Step-by-Step Implementation Guide

TDD。各ステップでテストを先に書き、`./scripts/process/build.sh` がテスト失敗で落ちることを見てから実装する。`go test` は直接実行しない。統合テストの前に必ずビルドする。

1. [x] **gitignore と設定**: `.gitignore` と `settings/decision-test.yaml` を追加する。
2. [x] **モジュールと logger**: `go.mod` を作る。`logger_test.go` を先に書き、ビルドが失敗することを確認し、`logger.go` を実装して `./scripts/process/build.sh` を通す。
3. [x] **config**: `config_test.go` の後に `config.go`。ビルド。
4. [x] **domain**: `validate_test.go` のテーブルを先に書く。失敗を確認してから `types.go` と `validate.go`。`testdata/account.json` と `email.json` はこのステップで置く（テストが読む）。ビルド。
5. [x] **prompt**: `prompt_test.go` のゴールデンが失敗することを確認し、`prompt.go` を実装。ビルド。
6. [x] **math と generate**: `math_test.go`、`generate_test.go` を先に書き、実装。ビルド。
7. [x] **engine**: `llamacpp_test.go` を先に書く。`httptest` の期待 JSON が落ちることを確認し、`engine.go` と `llamacpp.go` を実装。ビルド。
8. [x] **service**: `service_test.go` を先に書き、`service.go` を実装。ビルド。
9. [x] **api**: `api_test.go` を先に書き、`api.go` を実装。422 のケースで Engine が呼ばれないことをテストが証明する。ビルド。
10. [x] **cli**: `decide_test.go` を先に書き、`decide.go` を実装。ビルド。
11. [x] **main**: `cmd/decision-test/main.go` だけを `main` にする。`./scripts/process/build.sh` が `bin/decision-test.exe`（Windows）を出力することを確認する。
12. [x] **setup スクリプト**: 3 本を追加し、それぞれ `--help` が 0 で終わることを実行して確認する。モデル取得と llama-server 起動は統合テストの前提であり、このステップで実行してよい。失敗したら統合テストに進まない。
13. [x] **統合ランナー**: `integration_test.sh` に `-tags integration` を足す。
14. [x] **統合テストコード**: `tests/go.mod` と `tests/decision_systemone_test.go` を書く。
15. [x] **Verification Plan** を上から実行する。

## Verification Plan

### Automated Verification

1. **Build & Unit Tests**:

```bash
./scripts/process/build.sh
```

`bin/decision-test.exe` が存在すること。feature の単体テストが全て成功すること。

2. **Integration Tests**:

事前に次が成功していること。失敗したまま統合テストを開始しない。

```bash
./scripts/setup/install_llama_cpp.sh
./scripts/setup/download_model.sh
./scripts/setup/run_llama_server.sh
```

`run_llama_server.sh` は前景で動く。別シェルで次を実行する。

```bash
./scripts/process/build.sh && ./scripts/process/integration_test.sh --specify "TestDecisionSystemOne"
```

*   **Log Verification**: `TestDecisionSystemOne_BothOrder` が読むログに `level=DEBUG`、`component=decision`、`msg=direct completed`、その後 `msg=generation started`、`question_id=queue` があること。`ERROR` が同じログに無いこと。llama-server 未起動で `SKIP` ではなく `Fatal` になること。

このリポジトリの `integration_test.sh` は `--categories` を持たない。選択実行は `--specify "TestDecisionSystemOne"` のみ。全テスト一括は完了条件にしない。

3. **E2E Tests**:

GUI E2E は作らない。フロントエンドが無く、`tests/agentservice_e2e_test.go` も存在しない。代わりに上記 `tests/decision_systemone_test.go` が、ビルド済みバイナリと実 llama-server と実 GGUF を使ってシナリオ 1〜4 を自動実行する。手動 curl は代替にしない。

#### [NEW] `tests/decision_systemone_test.go`

*   **テストケース**: `TestDecisionSystemOne_Health`、`TestDecisionSystemOne_DirectAccount`、`TestDecisionSystemOne_GenerationEmail`、`TestDecisionSystemOne_BothOrder`。
*   **検証ポイント**: health の 20 ラベル、direct の確率和と `output_tokens==1`、generation のキー完全一致と和 1±0.02、both の ratio とログ順序。

### テスト項目のセルフレビュー

ボトムアップ順は、math / generate（末端）→ prompt → engine の HTTP 形 → service の合成 → api → cli → 実モデル統合、である。上位は下位が単体で通ったあとだけ意味を持つ。

| # | 観点 | 対応 |
| :--- | :--- | :--- |
| 1 | 正常系 | direct、generation、both を単体の偽物と統合の実モデルで確認 |
| 2 | 異常系・境界 | criteria 1 と 21、空 state、score、ラベル欠落 502、生成 JSON の和 1.02 と 1.021、同率 argmax |
| 3 | 外部連携 | 統合テストだけが実 llama-server と GGUF を使う。単体は httptest |
| 4 | データの一貫性 | 期待キーと criteria 順、softmax の和、ratio の除算 |
| 5 | 状態遷移 | health が warmup 前 503、後 200。both のログ順 |
| 6 | 設定の反映 | model 省略時は YAML の ID。`--method` がボディを上書き |
| 7 | 副作用 | 422 で Engine 呼び出し 0。Git に gguf を置かない |

セルフレビュー:

1. **網羅性**: シナリオ 5 は api 単体で証拠が取れる。シナリオ 2〜4 は統合テストが確率の和とキーと時間を見るので、「200 が返る」だけではない。シナリオ 6 は cli 単体が文字列を固定する。
2. **証拠**: choice が key 集合に含まれること、和、トークン数、ログ順まで断言する。正解の中身（必ず account_access）は仕様が要求していないので断言しない。
3. **迂回**: engine 単体が送った JSON の `top_logprobs` と `enable_thinking` を見るので、別経路で確率を作っても単体は落ちる。422 は呼び出し回数 0 で証明する。
4. **依存**: service テストは math が正しいことを前提に偽物 logprob を渡す。api テストは service を通す。統合はそれらがビルド済みである前提で、バイナリが古いと失敗する。だから Verification は `build.sh && integration_test.sh` の順である。

この項目が全部成功すれば、direct が 1 トークン読み出しであること、generation が JSON 検証付きであること、CLI が同じ API を叩くこと、まで言い切れる。実モデルの品質が Jev と一致することは言い切れない。仕様もそれを要求していない。

### 総合判定プロセス

全テスト成功後、実行者は次をログから確認し、実装計画の末尾ではなく実行時の報告に書く。

| # | チェック | 見ること |
| :--- | :--- | :--- |
| 1 | スキップ | `SKIP` が無い。未起動は `Fatal` |
| 2 | 部分エラー | 成功したテストのログに `ERROR` が無い |
| 3 | 迂回 | 422 テストの Engine 回数、direct リクエストの `max_tokens` |
| 4 | 設定 | health の `model` が `minicpm5-2b-q4_k_m` |
| 5 | 順序依存 | 4 本の統合テストがファイル内でサーバを各自起動し、互いの状態を見ない |
| 6 | カバレッジ | 新パッケージに対応する `_test.go` がビルドログに出ている |
| 7 | 外部 | 統合の前に llama-server の `/health` が成功している |

判定テンプレ（実行時に埋める）:

```markdown
### 総合判定結果
判定: 動作確認完了 / 条件付き確認完了 / 追加確認必要
全テスト数 / 成功 / 失敗 / 事実上スキップ:
チェック 1〜7:
判定理由:
```

「全テスト成功」だけを動作確認完了にしない。

## Documentation

`prompts/specifications` は存在しない。既存ドキュメントの更新は無い。仕様書 `prompts/phases/000-foundation/branches/main/ideas/000-DecisionTest.md` は本計画の入力であり、計画作成では編集しない。
