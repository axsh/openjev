# 003-DecisionWorkerPool

> **Source Specification**: `prompts/phases/000-foundation/branches/main/ideas/003-DecisionWorkerPool.md`

## Goal Description

`POST /v1/systemone` の 1 リクエストに含まれる N 質問を、プロセスに 1 つのワーカープール（Request Queue + `workers` 本の goroutine）で並列に処理する。ハンドラは質問を出現順にタスク化して投入し、ハンドラ専用の Response Queue から回収して `answers` を組み立てる。自分の投入数・回収数が `stall_timeout_ms` の間どちらも増えなければ切り上げ、回収済みの回答だけで HTTP 200 を返す。エンジンのセマフォ（`llama_parallel`）は撤去し、ワーカー数を llama への同時呼び出しの唯一の上限にする。`instructions` は string / object / array を受理する。`load` に `--questions` と `--workers` を足し、質問数 × スロット数（1〜6 × 1,5,10,15,20,30、ワーカー 30）のベンチマークを統合テスト `TestDecisionSystemOne_Batch` で回す。

## User Review Required

None.（依頼者は `/create-implementation-plan -> /execute-implementation-plan` の連続実行を指示済み。以下は計画側の判断で、仕様の範囲内で決めた点）

- `prompt.Messages` の第 2 引数は `string` のまま残し、`Text` → 文字列の変換は `Service.one` で行う。仕様の変更ファイル表は「`instructions` を `Text` から受ける」と書くが、プロンプト組み立ての責務を変えないほうが差分が小さい。
- `Service.Run` は `Pool` が nil のときエラー `worker pool is not started` を返す（暗黙に直列実行へ落とさない）。`StallTimeout` が 0 以下のときは既定 15 秒を使う（設定は 1 以上を保証するが、`Service` を直接組む単体テストの安全弁）。

## Requirement Traceability

| Requirement (from Spec) | Implementation Point (Section/File) |
| :--- | :--- |
| 1 対象と非対象（`-c 2048`、`cache_prompt: false`、非同期なし、`run_llama_server.sh` 不変） | 全体。`llamacpp.go` の `CachePrompt: false` は維持。`run_llama_server.sh` は触らない |
| 2 プロセスに 1 つの Request Queue（容量 1024 定数） | Proposed Changes > `internal/decision/pool.go` `requestQueueCapacity` |
| 2 `workers` 設定（省略/0 → 16、負はエラー） | `internal/config/config.go`、`settings/decision-test.yaml` |
| 2 起動時に `workers` 本の goroutine、停止時に止める | `pool.go` `Start`、`cmd/decision-test/main.go` `buildServer` / `runtime.shutdown` |
| 2 タスク = 質問 1 件、`both` は同じワーカーで direct → generation | `pool.go` `task`、`service.go` `one`（既存ロジックをそのまま executor にする） |
| 2 ctx 終了済みタスクは実行せず捨てる（DEBUG、結果を送らない） | `pool.go` ワーカーループ |
| 2 Response Queue 容量 = 質問数、送信は決してブロックしない | `service.go` `Run` の `make(chan result, len(req.Questions))` |
| 2 セマフォ撤去、`NewClient` 第 3 引数は `MaxIdleConnsPerHost`、`Health` はプール外 | `internal/engine/llamacpp.go` |
| 2 `direct_ms` が llama 側の待ちを含む旨を README に書く | Documentation > `README.md` |
| 3 422 検証はキュー投入前 | `internal/api/api.go`（既存の `domain.Validate` → `svc.Run` 順を維持） |
| 3 出現順に投入、回収順は問わない、`answers` を ID で組む | `service.go` `Run` |
| 3 `usage` は回収分の合計、`elapsedMs` はハンドラ入口から | `service.go` `Run`、`api.go`（既存） |
| 3 `ErrEngine` が 1 件でも回収されたら残りを取り消して 502 | `service.go` `Run`（`return resp{}, r.err`、`defer cancel()`）、`api.go`（既存の 502 マッピング） |
| 3 ハンドラが戻るとき ctx を cancel、実行中の llama 呼び出しは中断 | `service.go` `Run` の `context.WithCancel` + `defer cancel()`、`llamacpp.go` の `NewRequestWithContext`（既存） |
| 3 クライアント切断で回収を打ち切る | `service.go` `Run` の `case <-ctx.Done()` |
| 4 `submitted` / `received` の監視、増えたらタイマーを戻す | `service.go` `Run` の `progress()` |
| 4 停滞時は投入を止め、回収済みだけで 200、欠けた ID は `answers` に含めない | `service.go` `Run` の `case <-timer.C` |
| 4 `stall_timeout_ms`（省略/0 → 15000、負はエラー） | `config.go`、`settings/decision-test.yaml` |
| 4 WARN `systemone stalled`（`questions`, `submitted`, `received`, `missing_ids`, `stall_timeout_ms`） | `service.go` `Run` |
| 4 ハンドラ単位の判定と「欠けた 200」の性質を README に書く | Documentation > `README.md` |
| 5 `/health` に `workers` と `queue_depth` | `internal/domain/types.go` `Health`、`main.go` の health クロージャ |
| 6 `instructions` string / object / array、空・null・欠落は `instructions must not be empty`、数値・真偽値は `instructions must be a string, object, or array` | `internal/domain/types.go` `Text`、`validate.go` `parseQuestion` / `Validate` |
| 6 `marshalRequest` は `instructions` の元 JSON を書き戻す | `validate.go` `marshalRequest` |
| 7 `server starting` INFO に `workers`, `stall_timeout_ms` | `main.go` |
| 7 `systemone completed` DEBUG（`answered`, `missing`, `duration_ms`） | `service.go` `Run` |
| 7 `direct completed` / `generation started` DEBUG は残す | `service.go` `one`（既存） |
| 7 skip は DEBUG、ctx cancel の失敗は ERROR にしない | `pool.go`（skip）、`llamacpp.go`（非 2xx のみ ERROR、既存） |
| 8 `--questions n`（0 = そのまま、先頭 n 件、超過はエラー、負はエラー） | `validate.go` `TakeQuestions`、`internal/cli/load.go`、`main.go` |
| 8 `--workers m`（ラベル、既定 0） | `load.go`、`main.go` |
| 8 集計 `workers`, `questions`, `answers`, `answers_missing`, `questions_per_sec`（JSON と人間可読） | `load.go` `LoadReport`、`summarize`、`writeLoad` |
| 8 終了コードは `errors` 0 かつ `answers_missing` 0 のときだけ 0、stderr に `answers_missing N` | `load.go` `Load` |
| 9 `playground.json`（Playground 生成 JSON） | `features/decision-test/testdata/playground.json` |
| 9 `bank.json`（30 質問、c/s/n 交互、各プロンプト 256 トークン以内） | `features/decision-test/testdata/bank.json`、`validate_test.go` `TestBankFixture` |
| 10 `settings/decision-test.yaml`（`llama_parallel` 削除、`workers: 16`、`stall_timeout_ms: 15000`）、残っていても読まない | `settings/decision-test.yaml`、`config_test.go` `TestLoadIgnoresLlamaParallel` |
| 10 README（設定表、`load` の説明、フィクスチャ一覧、`direct_ms`、欠けた 200） | Documentation > `README.md` |
| 11 `TestServiceBothOrder` を質問ごとの read → generate に | `service_test.go` |
| 11 `TestDecisionSystemOne_Mixed` を ID ごとの順序に | `tests/decision_systemone_test.go` |
| 11 `TestSerializeRequests` / `TestTwoSlotOverlap` 撤去、同時数はプールの単体テストへ | `llamacpp_test.go`（削除 + `TestConcurrentReadsNotSerialized`）、`pool_test.go` |
| 11 `TestDecisionSystemOne_Load` は `workers: N`、既定 `stall_timeout_ms`、`answers_missing` 正は失敗 | `tests/decision_systemone_test.go` `writeOwnedConfig`、`runLoad` |
| 12 スロット 1〜6 × n = 1,5,10,15,20,30、ワーカー 30、concurrency 1、36 回の合否と health、ログ | `tests/decision_systemone_test.go` `TestDecisionSystemOne_Batch` |
| 12 スロット 3, 5, 6 の失敗はスキップせず失敗 | 同テスト（`t.Fatal`、`t.Skip` 不使用） |
| 任意要件（`cache_prompt: true`、`--kv-unified`、`/completion` バッチ、429、欠け ID フィールド、generation 負荷） | 実装しない（仕様で明示的に別仕様へ） |

## Proposed Changes

依存関係順: `domain`（型） → `engine` / `config` → `decision`（プール、サービス） → `api` → `cli` → `cmd` → `tests` → 設定と README。各コンポーネントで `_test.go` を先に書く。

### features/decision-test — internal/domain

#### [MODIFY] [validate_test.go](file://features/decision-test/internal/domain/validate_test.go)
*   **Description**: `instructions` の型、`TakeQuestions`、`QuestionCount`、フィクスチャ検証のテストを追加する。
*   **Technical Design**:
    *   `TestValidate` のテーブルに追加:
        | name | raw | wantErr / check |
        | --- | --- | --- |
        | `instructions_object` | `{"state":"hello","questions":{"q":{"type":"choice","instructions":{"ask":"Which queue?","hint":"billing or access"},"criteria":{"a":"A","b":"B"}}}}` | check: `req.Questions[0].Instructions.PromptText()` が `{"ask":"Which queue?","hint":"billing or access"}` |
        | `instructions_array` | `"instructions":["Which queue?","Pick one"]` | check: PromptText が `["Which queue?","Pick one"]` |
        | `instructions_number` | `"instructions":123` | wantErr `instructions must be a string, object, or array` |
        | `instructions_bool` | `"instructions":true` | wantErr `instructions must be a string, object, or array` |
        | `instructions_null` | `"instructions":null` | wantErr `instructions must not be empty` |
        | `instructions_missing` | `instructions` キー無し | wantErr `instructions must not be empty` |
        | `instructions_blank` | `"instructions":"   "` | wantErr `instructions must not be empty` |
        | 既存 `empty_instructions` | `""` | wantErr `instructions must not be empty`（substring `instructions` のまま通る） |
    *   `TestApplyMethodKeepsInstructionsJSON`: object instructions の入力に `ApplyMethod(raw, "generation")` を掛け、出力に `"instructions":{"ask":"Which queue?","hint":"billing or access"}` がそのまま含まれること。
    *   `TestTakeQuestions`: `bank.json` を読み、テーブル駆動で n = 0（出力が入力と同一バイト列）、1（`QuestionCount` が 1、ID `queue`）、5（ID 列が `queue, frustration, is_urgent, channel, severity`）、30（30 件）、31（エラーに `input has 30 questions; --questions 31 exceeds it`）、-1（エラー）。切り詰め後の本文が `Validate` を通ること。
    *   `TestBankFixture`: `bank.json` が `Validate` を通り、30 件、ID が一意、`Type` が `choice, score, noul` の繰り返し（`i % 3`）であること。
    *   `TestPlaygroundFixture`: `playground.json` が `Validate` を通り、ID が `department, urgency, wants_refund`、型が `choice, score, noul` であること。
*   **Logic**: 既存テーブルの `check` 関数の形に合わせる。フィクスチャは `os.ReadFile(filepath.Join("..", "..", "testdata", name))`。

#### [MODIFY] [types.go](file://features/decision-test/internal/domain/types.go)
*   **Description**: `State` を汎用の `Text` にし、`Question.Instructions` を `Text` にする。`Health` にフィールドを足す。
*   **Technical Design**:
    ```go
    // Text holds a JSON string, object, or array and renders it for the prompt.
    type Text struct {
    	raw json.RawMessage
    }

    // State is the shared context of a request; it follows the Text rules.
    type State = Text

    // TextOf wraps a plain string as Text (used by tests and callers that build requests in Go).
    func TextOf(s string) Text

    type Question struct {
    	ID           string
    	Type         string
    	Instructions Text
    	Criteria     []Criterion
    	criteriaRaw  json.RawMessage
    	sawCriteria  bool
    }

    type Health struct {
    	Ready          bool           `json:"ready"`
    	Model          string         `json:"model"`
    	LlamaReachable bool           `json:"llama_reachable"`
    	LabelTokenIDs  map[string]int `json:"label_token_ids"`
    	Workers        int            `json:"workers" doc:"Configured worker count of the decision pool."`
    	QueueDepth     int            `json:"queue_depth" doc:"Tasks currently waiting in the request queue."`
    	Error          string         `json:"error,omitempty"`
    }
    ```
*   **Logic**: `TextOf` は `json.Marshal(s)` の結果を `raw` に入れる。`Request`、`Response`、`Answer`、`Generation`、`Timings`、`Usage` は変えない。

#### [MODIFY] [validate.go](file://features/decision-test/internal/domain/validate.go)
*   **Description**: `Text` の共通ロジック、`instructions` の検証、`marshalRequest` の書き戻し、`TakeQuestions` / `QuestionCount`。
*   **Technical Design**:
    ```go
    var (
    	errTextEmpty = errors.New("text is empty")
    	errTextType  = errors.New("text must be a string, object, or array")
    )

    func (t *Text) UnmarshalJSON(b []byte) error // former State.UnmarshalJSON
    func (t Text) PromptText() (string, error)   // returns errTextEmpty or errTextType
    func (t Text) Raw() json.RawMessage           // trimmed raw JSON, nil when unset

    // TakeQuestions keeps the first n questions of a request body in appearance order.
    // n == 0 returns a copy of raw unchanged.
    func TakeQuestions(raw []byte, n int) ([]byte, error)

    // QuestionCount returns the number of questions in a request body.
    func QuestionCount(raw []byte) (int, error)
    ```
*   **Logic**:
    *   `Text.PromptText`: 空・`null` → `errTextEmpty`。先頭 `"` → 文字列に decode、`strings.TrimSpace` が空なら `errTextEmpty`、それ以外はその文字列。先頭 `{` または `[` → `string(raw)`。それ以外（数値、真偽値）→ `errTextType`。
    *   `Validate` の `state` 検査は現状どおり `state is empty` を返す（型が数値でも同じ文言。000 の互換）。
    *   `Validate` の質問ループ: `q.Instructions.PromptText()` のエラーが `errTextType` なら `instructions must be a string, object, or array`、それ以外は `instructions must not be empty`。この検査は `interpretQuestion` の前。
    *   `parseQuestion` の `case "instructions"`: `var raw json.RawMessage; dec.Decode(&raw)` → `q.Instructions.raw = raw`。文字列以外でもここでは失敗させない。
    *   `marshalRequest`: `"instructions":` の後に `q.Instructions.Raw()` をそのまま書く。未設定なら `null`。
    *   `TakeQuestions`: `n < 0` → `fmt.Errorf("questions must be >= 0")`。`n == 0` → `append([]byte(nil), raw...)`。それ以外は `parse(raw)` → `len(req.Questions) < n` なら `fmt.Errorf("input has %d questions; --questions %d exceeds it", len(req.Questions), n)` → `req.Questions = req.Questions[:n]` → 各質問に `interpretQuestion`（`marshalRequest` が `Criteria` を使うため）→ `marshalRequest(req)`。
    *   `QuestionCount`: `parse(raw)` して `len(req.Questions)`。

### features/decision-test — internal/prompt

#### [MODIFY] [prompt_test.go](file://features/decision-test/internal/prompt/prompt_test.go)
*   **Description**: object instructions のゴールデンを足す。
*   **Technical Design**: `TestObjectInstructions`: `Messages(accountState, `{"ask":"Which queue?","hint":"billing or access"}`, accountCriteria(), domain.MethodDirect, "choice")` の user 本文に `"\n\nQuestion:\n{\"ask\":\"Which queue?\",\"hint\":\"billing or access\"}\n\nAllowed options:\n"` が含まれること。
*   **Logic**: `prompt.go` は変更しない（第 2 引数は `string` のまま。変換は `Service.one`）。

### features/decision-test — internal/engine

#### [MODIFY] [llamacpp_test.go](file://features/decision-test/internal/engine/llamacpp_test.go)
*   **Description**: セマフォのテストを撤去し、直列化されないことと接続プール設定のテストを足す。
*   **Technical Design**:
    *   削除: `TestSerializeRequests`、`TestTwoSlotOverlap`。未使用になる import（`sync`）を整理する。
    *   `TestConcurrentReadsNotSerialized`: httptest ハンドラが `inFlight` を数え、4 本目が入るまで（または 2 秒で）待ってから応答する。`NewClient(srv.URL, nil, 4)` で 4 goroutine が同時に `ReadLabelLogprobs`。`maxSeen == 4`。`Health` はハンドラが `/health` を即応答するのを 100 ms 以内に返す（プール外のまま）。
    *   `TestClientIdleConns`: `NewClient(u, nil, 7).http.Transport.(*http.Transport).MaxIdleConnsPerHost == 7`、`NewClient(u, nil, 0)` は 1。
    *   既存テストの `NewClient(srv.URL, nil, 1)` はそのまま通る。
*   **Logic**: 「4 本目が入るまで待つ」は `sync.WaitGroup` ではなく、ハンドラ内で `atomic` のカウンタが 4 になるまで 10 ms ごとにポーリングし、上限 2 秒で諦めて応答する（デッドロックさせない）。

#### [MODIFY] [llamacpp.go](file://features/decision-test/internal/engine/llamacpp.go)
*   **Description**: セマフォ撤去。接続プールをワーカー数に合わせる。
*   **Technical Design**:
    ```go
    type Client struct {
    	baseURL string
    	http    *http.Client
    	log     *logger.Logger
    }

    // NewClient builds a llama-server client. maxIdleConns bounds idle keep-alive
    // connections per host and should match the worker count; values below 1 become 1.
    func NewClient(baseURL string, log *logger.Logger, maxIdleConns int) *Client
    ```
*   **Logic**:
    *   `sem`、`acquire`、`release` を削除。`ReadLabelLogprobs`、`Generate`、`Warmup` の先頭の `acquire` / `defer release` を削除。
    *   `transport := http.DefaultTransport.(*http.Transport).Clone()`; `transport.MaxIdleConnsPerHost = maxIdleConns`; `MaxIdleConns` が `maxIdleConns` 未満なら `maxIdleConns` にする。`http.Client{Timeout: 10 * time.Minute, Transport: transport}`。
    *   `CachePrompt: false`、grammar、logit_bias、`enable_thinking: false`、ERROR ログの条件（非 2xx のみ）は変えない。

### features/decision-test — internal/config

#### [MODIFY] [config_test.go](file://features/decision-test/internal/config/config_test.go)
*   **Description**: 新キーの既定値・検証と、旧キーの無視。
*   **Technical Design**:
    *   `TestLoad` の本文に `workers: 16` と `stall_timeout_ms: 15000` を足し、`cfg.Workers == 16`、`cfg.StallTimeoutMs == 15000` を確認。
    *   `TestLoadDefaults`: 両キーを省いた本文で `Workers == 16`、`StallTimeoutMs == 15000`。
    *   `TestLoadRejectsNegative`: テーブル `workers: -1` → エラーに `workers must be >= 1`、`stall_timeout_ms: -5` → `stall_timeout_ms must be >= 1`。
    *   `TestLoadIgnoresLlamaParallel`: `llama_parallel: 3` を含む本文がエラーにならず `Workers == 16`。
*   **Logic**: 本文は `TestLoad` の既存テンプレートを関数 `baseYAML(extra string) []byte` にして再利用する。

#### [MODIFY] [config.go](file://features/decision-test/internal/config/config.go)
*   **Technical Design**:
    ```go
    type File struct {
    	APIHost        string `yaml:"api_host"`
    	APIPort        int    `yaml:"api_port"`
    	LlamaURL       string `yaml:"llama_url"`
    	LlamaHost      string `yaml:"llama_host"`
    	LlamaPort      int    `yaml:"llama_port"`
    	LlamaBinary    string `yaml:"llama_binary"`
    	LlamaBuild     string `yaml:"llama_build"`
    	LlamaZip       string `yaml:"llama_zip"`
    	CudaRTZip      string `yaml:"cudart_zip"`
    	ModelPath      string `yaml:"model_path"`
    	ModelID        string `yaml:"model_id"`
    	ModelURL       string `yaml:"model_url"`
    	LogPath        string `yaml:"log_path"`
    	Workers        int    `yaml:"workers"`
    	StallTimeoutMs int    `yaml:"stall_timeout_ms"`
    }

    const (
    	DefaultWorkers        = 16
    	DefaultStallTimeoutMs = 15000
    )
    ```
*   **Logic**: `LlamaParallel` と `llama_parallel` の検証を削除。`Workers < 0` → `fmt.Errorf("workers must be >= 1")`、`== 0` → `DefaultWorkers`。`StallTimeoutMs < 0` → `fmt.Errorf("stall_timeout_ms must be >= 1")`、`== 0` → `DefaultStallTimeoutMs`。yaml.v3 は未知キーを無視するので `llama_parallel` は読まれない。

### features/decision-test — internal/decision

#### [NEW] [pool_test.go](file://features/decision-test/internal/decision/pool_test.go)
*   **Description**: プール単体。executor はフェイク関数。
*   **Technical Design**:
    *   `TestPoolMaxConcurrency`: executor は `inFlight` を `atomic` で数え、`release` チャネルが閉じるまで待ってから `domain.Answer{Type: "choice"}` を返す。`newPool(4, exec, logger.New(io.Discard))` を `t.Context()` で `Start`。質問 ID `q00..q09` の 10 タスクを容量 10 の results に向けて投入。`inFlight` が 4 になるまで最大 2 秒待ち、`maxSeen == 4` かつ `Depth() == 6`。`close(release)` 後、results から 10 件回収し ID 集合が一致。
    *   `TestPoolSkipsCancelledTask`: cancel 済み ctx のタスクと、生きた ctx のタスクを順に投入。生きたタスクの結果が返った後、executor 呼び出し回数は 1、results に他の要素は無い（`len(results) == 0`）。ログバッファに `task skipped` と `question_id=dead`。
    *   `TestPoolStopsOnContext`: `Start(ctx)` の ctx を cancel した後、投入したタスクが 200 ms 以内に実行されない（executor 呼び出し 0）。
*   **Logic**: 待ち合わせは `time.After` 付きの `select` で 3 秒以内に必ず終える。

#### [NEW] [pool.go](file://features/decision-test/internal/decision/pool.go)
*   **Technical Design**:
    ```go
    // requestQueueCapacity bounds the singleton request queue. A full queue blocks
    // the submitting handler, which its own stall timer covers.
    const requestQueueCapacity = 1024

    type task struct {
    	ctx      context.Context
    	state    string
    	question domain.Question
    	method   domain.Method
    	results  chan<- result
    }

    type result struct {
    	id     string
    	answer domain.Answer
    	used   usage
    	err    error
    }

    // executor runs one question. Service.one satisfies it.
    type executor func(ctx context.Context, state string, question domain.Question, method domain.Method) (domain.Answer, usage, error)

    // Pool is the process-wide worker pool that answers questions from the request queue.
    type Pool struct {
    	queue   chan task
    	exec    executor
    	log     *logger.Logger
    	workers int
    }

    func newPool(workers int, exec executor, log *logger.Logger) *Pool
    func (p *Pool) Start(ctx context.Context)
    func (p *Pool) Depth() int
    func (p *Pool) Workers() int
    func (p *Pool) worker(ctx context.Context, id int)
    ```
*   **Logic**:
    *   `newPool`: `workers < 1` は 1 に丸める。`queue = make(chan task, requestQueueCapacity)`。
    *   `Start`: `for i := range p.workers { go p.worker(ctx, i) }`。DEBUG `worker pool started` に `workers`, `queue_capacity`。
    *   `worker`: ループで `select { case <-ctx.Done(): return; case t := <-p.queue: ... }`。`t.ctx.Err() != nil` なら DEBUG `task skipped`（`question_id`, `worker_id`）で `continue`（結果は送らない）。それ以外は `answer, used, err := p.exec(t.ctx, t.state, t.question, t.method)` → `t.results <- result{id: t.question.ID, answer: answer, used: used, err: err}`。`err != nil && t.ctx.Err() != nil` のときは DEBUG `task cancelled`（ERROR にしない）。
    *   `Depth`: `len(p.queue)`。

#### [MODIFY] [service_test.go](file://features/decision-test/internal/decision/service_test.go)
*   **Description**: フェイクエンジンを質問識別・ブロック・遅延に対応させ、プール経由の `Run` を検証する。
*   **Technical Design**:
    ```go
    type fakeEngine struct {
    	mu       sync.Mutex
    	calls    []string          // "read:<question>" / "generate:<question>"
    	text     string
    	emptyTop bool
    	blockOn  string            // question text whose read blocks until ctx is done
    	unblocked chan struct{}    // closed when the blocked read observes ctx.Done
    	delay    time.Duration     // sleep per read
    }
    func questionOf(messages []prompt.Message) string // text between "Question:\n" and "\n\n" of the user message
    func newTestService(t *testing.T, eng engine.Engine, workers int, stall time.Duration, logs io.Writer) *Service
    ```
    *   `testQuestion()` は `Instructions: domain.TextOf("Which queue?")`。`questionWithID(id, text string) domain.Question` を足す。
    *   `TestServiceDirect`: `newTestService(t, fake, 2, time.Second, io.Discard)`。既存の assert（calls が `read:Which queue?` 1 件、`choice == account_access`、`DirectMs > 0`、`OutputTokens == 1`）。
    *   `TestServiceBothOrder`（改訂）: 質問 `queue`（text `Which queue?`）と `next`（text `Which next?`）を `both` で実行、ワーカー 2。assert: calls は 4 件、質問ごとに `read:<q>` の index < `generate:<q>` の index、両 ID が `Answers` にある、`ratio == GenerationMs / DirectMs`。質問間の順序は見ない。
    *   `TestServiceGenerationInvalid`、`TestServiceMissingLogit`: `newTestService` に置き換えるだけ。`TestServiceMissingLogit` は質問 2 件で実行し、`errors.Is(err, ErrEngine)` と `missing option logit for A`。
    *   `TestServiceStallReturnsPartial`: 質問 `a`（`Which a?`）、`b`（`Block me`）、`c`（`Which c?`）、`blockOn: "Block me"`、ワーカー 3、`stall 200ms`、ログはバッファ。`start := time.Now()`; `Run` は err nil、`time.Since(start) >= 200ms`、`Answers` のキーは `a`, `c` のみ、`Usage.OutputTokens == 2`。ログに `systemone stalled`、`questions=3`、`submitted=3`、`received=2`、`missing_ids=b`、`stall_timeout_ms=200`。`Run` 復帰後 `fake.unblocked` が 1 秒以内に閉じる（ctx cancel が伝わった）。
    *   `TestServiceProgressResetsTimer`: `delay: 150ms`、ワーカー 1、`stall 200ms`、質問 5 件（`q1..q5`）。`Run` は err nil、`Answers` 5 件、経過 ≥ 750 ms、ログに `systemone stalled` が無い、`systemone completed` に `answered=5 missing=0`。
    *   `TestServiceWithoutPool`: `Pool` nil の `Service.Run` がエラー `worker pool is not started`。
    *   `TestApplyNoulOmitsConfidence` は変更なし。
*   **Logic**: `fakeEngine.ReadLabelLogprobs` は `q := questionOf(messages)`; `mu` で `calls` に追記; `blockOn == q` なら `<-ctx.Done()` → `close(unblocked)`（`sync.Once`）→ `return ReadResult{}, ctx.Err()`; `delay > 0` なら `time.Sleep`; 既存の top 生成。`Generate` も `calls` に `generate:<q>` を追記。

#### [MODIFY] [service.go](file://features/decision-test/internal/decision/service.go)
*   **Description**: `Run` をプール投入・回収・停滞判定に置き換える。`one` は executor として残し、`instructions` を `Text` から文字列にする。
*   **Technical Design**:
    ```go
    const defaultStallTimeout = 15 * time.Second

    type Service struct {
    	Engine       engine.Engine
    	Labels       []engine.Label
    	Log          *logger.Logger
    	ModelID      string
    	Pool         *Pool
    	StallTimeout time.Duration
    }

    // StartPool creates the singleton pool backed by this service and starts its workers.
    func (s *Service) StartPool(ctx context.Context, workers int)

    func (s *Service) Run(ctx context.Context, req domain.Request) (domain.Response, error)
    func (s *Service) one(ctx context.Context, state string, question domain.Question, method domain.Method) (domain.Answer, usage, error)
    ```
*   **Logic** (`Run`):
    1. `state, err := req.State.PromptText()`（現状どおり）。`s.Pool == nil` → `errors.New("worker pool is not started")`。
    2. `stall := s.StallTimeout; if stall <= 0 { stall = defaultStallTimeout }`。
    3. `ctx, cancel := context.WithCancel(ctx); defer cancel()`。`results := make(chan result, len(req.Questions))`。`resp := domain.Response{Model: req.Model, Answers: map[string]domain.Answer{}}`。`submitted, received := 0, 0`。`timer := time.NewTimer(stall); defer timer.Stop()`。`progress := func() { if !timer.Stop() { select { case <-timer.C: default: } }; timer.Reset(stall) }`。`start := time.Now()`。
    4. `for received < len(req.Questions)`:
        *   `var submit chan<- task; var next task`。`submitted < len(req.Questions)` のとき `submit = s.Pool.queue`、`next = task{ctx, state, req.Questions[submitted], req.Method, results}`。
        *   `select`:
            *   `case submit <- next`: `submitted++`; `progress()`。
            *   `case r := <-results`: `received++`; `r.err != nil` → `return domain.Response{}, r.err`; `resp.Answers[r.id] = r.answer`; `resp.Usage.InputTokens += r.used.in`; `resp.Usage.OutputTokens += r.used.out`; `progress()`。
            *   `case <-timer.C`: `missing := IDs of req.Questions not in resp.Answers（出現順）`; `s.Log.Warn("systemone stalled", "questions", len(req.Questions), "submitted", submitted, "received", received, "missing_ids", strings.Join(missing, ","), "stall_timeout_ms", stall.Milliseconds())`; `return resp, nil`。
            *   `case <-ctx.Done()`: `return domain.Response{}, ctx.Err()`。
    5. ループ終了後 `s.Log.Debug("systemone completed", "questions", len(req.Questions), "answered", len(resp.Answers), "missing", len(req.Questions)-len(resp.Answers), "duration_ms", float64(time.Since(start).Microseconds())/1000)`; `return resp, nil`。
*   **Logic** (`one`): 先頭で `instructions, err := question.Instructions.PromptText(); if err != nil { return ..., fmt.Errorf("%w: %v", ErrEngine, err) }`（検証済みなので通常は到達しない）。`prompt.Messages(state, instructions, ...)` に渡す。それ以外の direct → generation の順、`applyDirect`、`applyGeneration`、DEBUG `direct completed` / `generation started` は変更なし。
*   **Logic** (`StartPool`): `s.Pool = newPool(workers, s.one, s.Log); s.Pool.Start(ctx)`。

### features/decision-test — internal/api

#### [MODIFY] [api_test.go](file://features/decision-test/internal/api/api_test.go)
*   **Description**: サービスにプールを付け、502 を 2 質問に広げ、health の新フィールドを確認する。
*   **Technical Design**:
    *   `newService(t *testing.T, eng *countingEngine) *decision.Service`: `StallTimeout: time.Second` を設定し `svc.StartPool(t.Context(), 4)`。全呼び出し箇所に `t` を渡す。
    *   `countingEngine.calls` は `atomic.Int32`（ワーカーから並列に呼ばれる）。
    *   `TestMissingLogit502`: 本文を 2 質問（`q1`, `q2` とも choice 2 options）にし、502 と `missing option logit for A`、`detail`。
    *   `TestHealthStatus`: health クロージャが `domain.Health{Ready: true, Model: ..., Workers: 16, QueueDepth: 0}` を返し、本文に `"workers":16` と `"queue_depth":0`。
    *   `TestRejectsBeforeEngine` に `instructions` が数値のケース `{"state":"hello","questions":{"q":{"type":"choice","instructions":123,"criteria":{"a":"A","b":"B"}}}}` を足す（422、engine 呼び出し 0）。
    *   `TestPlaygroundFixture`: `playground.json` を POST し 200、`answers` のキーが 3 つ、`department.type == "choice"`、`urgency.type == "score"`、`wants_refund.type == "noul"`。
*   **Logic**: `api.go` は変更しない。

### features/decision-test — internal/cli

#### [MODIFY] [load_test.go](file://features/decision-test/internal/cli/load_test.go)
*   **Description**: `--questions` の本文切り詰め、集計キー、欠けた回答と終了コード。
*   **Technical Design**:
    *   `TestLoadParallel`: 応答本文の `answers` を 1 件のまま、`report.Questions == 1`、`report.Answers == 10`、`report.AnswersMissing == 0`、`report.QuestionsPerSec > 0`、`report.Workers == 0`（未指定）を追加確認。
    *   `TestLoadTakesQuestions`: 3 質問の入力（`a`, `b`, `c`）を書き、httptest が受けた本文を `domain.QuestionCount` で数えて 2 であること（`Questions: 2`）、応答は `{"answers":{"a":{},"b":{}}}` を返し `report.Questions == 2`、`report.Answers == 2 * concurrency`、`AnswersMissing == 0`。`Workers: 30` で `report.Workers == 30`。
    *   `TestLoadCountsMissingAnswers`: 3 質問の入力、`Questions: 3`、`Concurrency: 2`、httptest は `{"answers":{"a":{},"b":{}}}`（2 件）を返す。`Load` はエラーを返し、`report.Success == 2`、`report.Answers == 4`、`report.AnswersMissing == 2`、stderr に `answers_missing 2`。
    *   `TestLoadQuestionsExceeds`: `Questions: 4` で 3 質問の入力 → `Load` がサーバへ送らずエラー（httptest のヒット数 0）。
    *   人間可読: `TestLoadHumanReport`: `JSON: false` で stdout に `questions: `、`answers: `、`answers_missing: `、`questions_per_sec: `、`workers: ` の行がある。
*   **Logic**: 入力書き出しは既存 `writeInput`。

#### [MODIFY] [load.go](file://features/decision-test/internal/cli/load.go)
*   **Technical Design**:
    ```go
    type LoadOptions struct {
    	Input       string
    	Server      string
    	Method      string
    	Concurrency int
    	Slots       int
    	Workers     int
    	Questions   int
    	JSON        bool
    }

    type LoadReport struct {
    	Slots           int            `json:"slots"`
    	Workers         int            `json:"workers"`
    	Concurrency     int            `json:"concurrency"`
    	Requests        int            `json:"requests"`
    	Questions       int            `json:"questions"`
    	Success         int            `json:"success"`
    	Errors          int            `json:"errors"`
    	Answers         int            `json:"answers"`
    	AnswersMissing  int            `json:"answers_missing"`
    	Statuses        map[string]int `json:"statuses"`
    	WallMs          float64        `json:"wall_ms"`
    	LatencyMs       LatencyMs      `json:"latency_ms"`
    	ThroughputRps   float64        `json:"throughput_rps"`
    	QuestionsPerSec float64        `json:"questions_per_sec"`
    	DirectMsSum     *float64       `json:"direct_ms_sum,omitempty"`
    }

    type loadResult struct {
    	status    int
    	latency   float64
    	answers   int
    	direct    float64
    	hasDirect bool
    	err       error
    }

    func inspectAnswers(raw []byte) (count int, direct float64, hasDirect bool) // replaces sumDirect
    ```
*   **Logic**:
    *   `Load`: `raw` 読み込み → `body, err := domain.TakeQuestions(raw, opt.Questions)`（0 はそのまま）→ `body, err = domain.ApplyMethod(body, opt.Method)` → `questions, err := domain.QuestionCount(body)`。以降は現状の goroutine 一斉解放。
    *   `oneLoad`: 成功時 `result.answers, result.direct, result.hasDirect = inspectAnswers(raw)`。
    *   `summarize(opt, questions, results, wall)`: `Workers = opt.Workers`、`Questions = questions`、成功ごとに `Answers += r.answers`。`AnswersMissing = Success*Questions - Answers`。`QuestionsPerSec = Answers / (wall/1000)`（wall 0 なら 0）。
    *   `Load` の終端: `report.Errors > 0` → stderr `errors N`; `report.AnswersMissing > 0` → stderr `answers_missing N`; どちらかが正なら `fmt.Errorf("errors %d answers_missing %d", ...)` を返す。
    *   `writeLoad` 人間可読: `slots`, `workers`, `concurrency`, `requests`, `questions`, `success`, `errors`, `answers`, `answers_missing`, `status_<code>`, `wall_ms`, `latency_*`, `throughput_rps`, `questions_per_sec`, あれば `direct_ms_sum` の順。

### features/decision-test — cmd/decision-test

#### [MODIFY] [main.go](file://features/decision-test/cmd/decision-test/main.go)
*   **Technical Design**:
    ```go
    type runtime struct {
    	srv      *http.Server
    	log      *logger.Logger
    	stopPool context.CancelFunc
    }

    func buildServer(opts *serverOptions) (*http.Server, *logger.Logger, context.CancelFunc, error)
    ```
*   **Logic**:
    *   `engine.NewClient(cfg.LlamaURL, log, cfg.Workers)`。
    *   ウォームアップ成功後: `svc = &decision.Service{..., StallTimeout: time.Duration(cfg.StallTimeoutMs) * time.Millisecond}`; `poolCtx, stopPool := context.WithCancel(context.Background())`; `svc.StartPool(poolCtx, cfg.Workers)`; INFO `server starting` に `workers`, `stall_timeout_ms` を追加。
    *   health クロージャ: `h.Workers = cfg.Workers`; `svc != nil && svc.Pool != nil` なら `h.QueueDepth = svc.Pool.Depth()`。
    *   `runtime.listen` は `stopPool` を保存。`shutdown`: `srv.Shutdown(ctx)` の後に `stopPool()`（nil チェック）。`buildServer` が失敗したときは `stopPool` は nil のまま。
    *   `loadCommand`: `cmd.Flags().IntVar(&questions, "questions", 0, "Keep only the first N questions of the input (0 = all)")`、`cmd.Flags().IntVar(&workers, "workers", 0, "Worker count recorded in the report")`。`questions < 0 || workers < 0` は実行前にエラー。`runLoad` に渡し `cli.LoadOptions{..., Workers: workers, Questions: questions}`。

### features/decision-test — testdata

#### [NEW] [playground.json](file://features/decision-test/testdata/playground.json)
*   **Description**: 仕様の背景に載せた Playground 生成 JSON をそのまま置く（`state` 1 件、`department` choice 3 options、`urgency` score 3 levels、`wants_refund` noul criteria 省略）。

#### [NEW] [bank.json](file://features/decision-test/testdata/bank.json)
*   **Description**: `state` は `account.json` と同一文（`A customer says a password reset succeeded, but every login attempt still returns ‘account locked’. Two unlock emails were requested and neither arrived.`）。`questions` は仕様 要件 9 の表の 30 件をこの順に、ID・type・instructions・criteria をそのまま JSON にする。choice は `{"key": "description"}`、score は配列、noul は省略または `{"true": "...", "false": "..."}`。

### tests

#### [MODIFY] [decision_systemone_test.go](file://tests/decision_systemone_test.go)
*   **Description**: Batch ベンチマーク、Mixed の順序緩和、Load の設定キー、Playground の追加。
*   **Technical Design**:
    ```go
    type loadReport struct {
    	Slots           int            `json:"slots"`
    	Workers         int            `json:"workers"`
    	Concurrency     int            `json:"concurrency"`
    	Requests        int            `json:"requests"`
    	Questions       int            `json:"questions"`
    	Success         int            `json:"success"`
    	Errors          int            `json:"errors"`
    	Answers         int            `json:"answers"`
    	AnswersMissing  int            `json:"answers_missing"`
    	Statuses        map[string]int `json:"statuses"`
    	WallMs          float64        `json:"wall_ms"`
    	LatencyMs       struct{ Min, P50, P95, Max float64 } `json:"latency_ms"`
    	ThroughputRps   float64        `json:"throughput_rps"`
    	QuestionsPerSec float64        `json:"questions_per_sec"`
    	DirectMsSum     *float64       `json:"direct_ms_sum"`
    }

    func writeOwnedConfig(t *testing.T, root string, workers int, logPath string) string // replaces writeSlotConfig
    func runLoad(t *testing.T, bin, input, server string, slots, workers, concurrency, questions int, watchHealth bool, wantWorkers int) loadReport
    func TestDecisionSystemOne_Batch(t *testing.T)
    func TestDecisionSystemOne_Playground(t *testing.T)
    ```
*   **Logic**:
    *   `writeOwnedConfig`: `api_port: 18199`、`llama_url: http://127.0.0.1:18280`、`llama_port: 18280`、`workers: <workers>`（無ければ追記）、`log_path`。`llama_parallel:` の行は削除する。`stall_timeout_ms` は既定に任せる。
    *   `runLoad`: 引数に `--workers`、`--questions`（0 なら付けない）を足す。`watchHealth` のとき、health 本文に `"ready":true` と `"workers":<wantWorkers>` があること。`waitErr != nil || report.Errors != 0 || report.AnswersMissing != 0` は `t.Fatalf`。
    *   `TestDecisionSystemOne_Load`: `writeOwnedConfig(t, root, slots, logPath)`。`runLoad(..., slots, slots, n, 0, n == 50, slots)`。assert に `report.Workers == slots`、`report.Questions == 1`、`report.Answers == n` を足す。
    *   `TestDecisionSystemOne_Batch`: `input = bank.json`。`for _, slots := range []int{1, 2, 3, 4, 5, 6}`: `startOwnedLlama(t, llamaBin, model, slots)`; `cfg := writeOwnedConfig(t, root, 30, logPath)`; `startOwnedAPI`; `for _, n := range []int{1, 5, 10, 15, 20, 30}`: `report := runLoad(t, apiBin, input, base, slots, 30, 1, n, n == 30, 30)`; `t.Logf("slots=%d questions=%d wall_ms=%.3f p50_ms=%.3f questions_per_sec=%.3f direct_ms_sum=%v", ...)`; assert `Slots == slots`, `Workers == 30`, `Concurrency == 1`, `Requests == 1`, `Questions == n`, `Success == 1`, `Errors == 0`, `Answers == n`, `AnswersMissing == 0`, `WallMs > 0`, `Statuses == {"200": 1}`。スロット終了後、API ログに `level=ERROR` と `systemone stalled` が無いこと。`stopProcess(api)`, `stopProcess(llama)`。
    *   `TestDecisionSystemOne_Mixed`: ログ検査を ID ごとに変える。各 `id` について `direct completed` かつ `question_id=<id>` を含む行の最初の位置 < `generation started` かつ `question_id=<id>` を含む行の最初の位置。両行に `question_type=<type>`。`level=ERROR` 無し。
    *   `TestDecisionSystemOne_Playground`: 利用者の llama（設定の `llama_url`）を使う。`startServer(t, root, 18189, "")`。`playground.json` を POST し、`answers` が `department`（`type == choice`、`choice` が 3 key のいずれか）、`urgency`（`type == score`、`score` が 0〜2）、`wants_refund`（`type == noul`、`noul` が 0〜1）。`usage.output_tokens == 3`。
    *   `TestDecisionSystemOne_BothOrder`（1 質問）は変更不要。

### settings / README

#### [MODIFY] [decision-test.yaml](file://settings/decision-test.yaml)
*   **Logic**: `llama_parallel: 1` を削除し、`workers: 16`、`stall_timeout_ms: 15000` を追加。

#### [MODIFY] [README.md](file://README.md)
*   Documentation 節を参照。

## Step-by-Step Implementation Guide

各ステップは「テストを書く → 失敗を確認 → 実装 → `./scripts/process/build.sh` が通る → コミット」。ビルドスクリプトは feature の全単体テストを走らせるので、ステップ途中の赤は同じステップ内で緑にしてからコミットする。

1. [x] **domain: `Text` と `instructions`、`TakeQuestions`、フィクスチャ**
    *   Add `features/decision-test/testdata/playground.json` and `features/decision-test/testdata/bank.json`.
    *   Edit `internal/domain/validate_test.go`: add the `instructions_*` table rows, `TestApplyMethodKeepsInstructionsJSON`, `TestTakeQuestions`, `TestBankFixture`, `TestPlaygroundFixture`.
    *   Edit `internal/prompt/prompt_test.go`: add `TestObjectInstructions`.
    *   Edit `internal/domain/types.go`: `Text`, `State = Text`, `TextOf`, `Question.Instructions Text`, `Health.Workers` / `QueueDepth`.
    *   Edit `internal/domain/validate.go`: `errTextEmpty` / `errTextType`, `Text.UnmarshalJSON` / `PromptText` / `Raw`, `parseQuestion` の `instructions` を raw に、`Validate` の文言分岐、`marshalRequest` の書き戻し、`TakeQuestions`、`QuestionCount`.
    *   Edit `internal/decision/service.go` `one`: `question.Instructions.PromptText()` を `prompt.Messages` に渡す（コンパイルを通す最小変更）。Edit `internal/decision/service_test.go` `testQuestion`: `domain.TextOf("Which queue?")`.
    *   Run `./scripts/process/build.sh`. Commit `feat(decision-test): accept object instructions and add question fixtures`.
2. [x] **engine + config + settings: セマフォ撤去と新設定キー**
    *   Edit `internal/engine/llamacpp_test.go`: remove `TestSerializeRequests` / `TestTwoSlotOverlap`, add `TestConcurrentReadsNotSerialized`, `TestClientIdleConns`.
    *   Edit `internal/engine/llamacpp.go`: remove `sem` / `acquire` / `release`, `NewClient(baseURL, log, maxIdleConns)` with cloned transport.
    *   Edit `internal/config/config_test.go`: `baseYAML`, `TestLoadDefaults`, `TestLoadRejectsNegative`, `TestLoadIgnoresLlamaParallel`.
    *   Edit `internal/config/config.go`: `Workers`, `StallTimeoutMs`, defaults, remove `LlamaParallel`.
    *   Edit `settings/decision-test.yaml`: replace `llama_parallel: 1` with `workers: 16` and `stall_timeout_ms: 15000`.
    *   Edit `cmd/decision-test/main.go`: `engine.NewClient(cfg.LlamaURL, log, cfg.Workers)`（この時点ではプール未導入。コンパイルを通す）。
    *   Run `./scripts/process/build.sh`. Commit `feat(decision-test): replace llama semaphore with worker-sized connection pool settings`.
3. [x] **decision: プールと `Run` の置き換え、api の追随**
    *   Add `internal/decision/pool_test.go` (`TestPoolMaxConcurrency`, `TestPoolSkipsCancelledTask`, `TestPoolStopsOnContext`).
    *   Edit `internal/decision/service_test.go`: new `fakeEngine`, `questionOf`, `newTestService`, revised `TestServiceBothOrder`, `TestServiceStallReturnsPartial`, `TestServiceProgressResetsTimer`, `TestServiceWithoutPool`, 2-question `TestServiceMissingLogit`.
    *   Add `internal/decision/pool.go`.
    *   Edit `internal/decision/service.go`: `Service.Pool` / `StallTimeout`, `StartPool`, new `Run`.
    *   Edit `internal/api/api_test.go`: `newService(t, eng)` with pool, atomic `calls`, 2-question 502, health fields, numeric instructions 422, `TestPlaygroundFixture`.
    *   Edit `cmd/decision-test/main.go`: pool start/stop, `StallTimeout`, health `workers` / `queue_depth`, INFO fields.
    *   Run `./scripts/process/build.sh`. Commit `feat(decision-test): answer questions of one request through a worker pool with stall cutoff`.
4. [x] **cli: `load --questions` / `--workers` と集計**
    *   Edit `internal/cli/load_test.go`: `TestLoadTakesQuestions`, `TestLoadCountsMissingAnswers`, `TestLoadQuestionsExceeds`, `TestLoadHumanReport`, extended `TestLoadParallel`.
    *   Edit `internal/cli/load.go`: options, report fields, `inspectAnswers`, `summarize`, exit rule, human lines.
    *   Edit `cmd/decision-test/main.go`: `--questions`, `--workers` flags and validation.
    *   Run `./scripts/process/build.sh`. Commit `feat(decision-test): measure questions per request in load`.
5. [x] **tests + README: Batch ベンチマークと退行の改訂**
    *   Edit `tests/decision_systemone_test.go`: `loadReport`, `writeOwnedConfig`, `runLoad`, `TestDecisionSystemOne_Load` update, `TestDecisionSystemOne_Batch`, `TestDecisionSystemOne_Mixed` per-ID order, `TestDecisionSystemOne_Playground`.
    *   Edit `README.md` (Documentation 節).
    *   Run `./scripts/process/build.sh`. Commit `test(decision-test): add question-count batch benchmark and relax per-question log order`.
6. [ ] **Verification Plan の実行**（下記）。統合テストの結果と総合判定を本計画の末尾に記録し、コミットする。

## Verification Plan

### Automated Verification

1.  **Build & Unit Tests**:
    ```bash
    ./scripts/process/build.sh
    ```
    *   全パッケージの単体テストが通り、`bin/decision-test.exe` が出る。停滞テストは 200 ms タイマー、進捗テストは 750 ms 程度で終わる（3 秒以内）。

2.  **Integration Tests — 質問数 × スロット数（自前で llama を 18280 に起動。他の llama-server は止めた状態で実行する）**:
    ```bash
    ./scripts/process/build.sh && ./scripts/process/integration_test.sh --specify "TestDecisionSystemOne_Batch|TestDecisionSystemOne_Load"
    ```
    *   **Log Verification**: `TestDecisionSystemOne_Batch` の 36 行の `t.Logf` に `slots` 1〜6、`questions` 1,5,10,15,20,30、`wall_ms > 0`、`questions_per_sec > 0`。API ログ（各スロットの `decision.log`）に `level=ERROR` と `systemone stalled` が無い。health が n = 30 の最中に 200 で `"workers":30`。`TestDecisionSystemOne_Load` の 12 行に `answers_missing` 0（`runLoad` が assert）。

3.  **Integration Tests — 退行（利用者が起動した llama-server が設定の `llama_url` にあること）**:
    ```bash
    ./scripts/process/build.sh && ./scripts/process/integration_test.sh --specify "TestDecisionSystemOne_DirectAccount|TestDecisionSystemOne_Mixed|TestDecisionSystemOne_Playground|TestDecisionSystemOne_BothOrder"
    ```
    *   **Log Verification**: `Mixed` で ID ごとに `direct completed` → `generation started` の順。`Playground` で 3 回答と `output_tokens == 3`。`level=ERROR` 無し。

4.  **E2E Tests (新規/追加)**:
    新機能は CLI サブコマンドと HTTP API なので統合テストは必須。既存ヘルパー（`startOwnedLlama`、`startOwnedAPI`、`startServer`、`postJSON`、`answer`、`stopProcess`）を再利用する。

    #### [MODIFY] [decision_systemone_test.go](file://tests/decision_systemone_test.go)
    *   **テストケース**: `TestDecisionSystemOne_Batch`（36 セル）、`TestDecisionSystemOne_Playground`（Playground JSON の 3 回答）、`TestDecisionSystemOne_Mixed`（ID ごとの順序）、`TestDecisionSystemOne_Load`（`workers: N`、`answers_missing` 0）。
    *   **検証ポイント**: 1 リクエストの n 質問が欠けなく 200 で返る（`answers == n`、`answers_missing == 0`）。プールが health を止めない（n = 30 の最中に 3 秒以内に 200 と `workers` 30）。停滞の切り上げが本番経路で起きていない（`systemone stalled` 無し）。002 の HTTP 同時数の受理が `workers = slots` で保たれる。

### テスト項目設計のセルフレビュー（testing-rules §11）

ボトムアップ順: `domain.Text` / `TakeQuestions`（末端） → `engine`（直列化されない） → `decision.Pool`（同時数・skip・停止） → `decision.Service.Run`（投入・回収・停滞・エラー） → `api`（200 / 422 / 502 / health） → `cli.Load`（本文と集計） → 統合（実 llama で 36 セル + 退行）。

| 観点 | 対応テスト |
| --- | --- |
| 正常系 | `TestServiceDirect`、`TestServiceBothOrder`、`TestPoolMaxConcurrency`、`TestLoadTakesQuestions`、`TestDecisionSystemOne_Batch`、`TestDecisionSystemOne_Playground` |
| 異常系・境界値 | `instructions_*`（数値・null・欠落・空白）、`TestTakeQuestions`（0、超過、負）、`TestLoadRejectsNegative`、`TestServiceWithoutPool`、`TestLoadQuestionsExceeds` |
| 外部連携の実動作 | `TestConcurrentReadsNotSerialized`（httptest）、統合の 36 セル（実 llama-server、スロット 1〜6） |
| データの一貫性 | `TestApplyMethodKeepsInstructionsJSON`（往復）、`TestBankFixture`（30 件・一意・型順）、`answers == n` |
| 状態遷移 | `TestServiceStallReturnsPartial`（タイマー発火 → 部分応答 → ctx cancel）、`TestServiceProgressResetsTimer`（進捗でタイマーが戻る）、`TestPoolStopsOnContext` |
| 設定・構成の反映 | `TestLoadDefaults`、`TestLoadIgnoresLlamaParallel`、health の `"workers":30`（統合） |
| 副作用 | `TestPoolSkipsCancelledTask`（結果を送らない、executor 未呼び出し）、統合の `level=ERROR` / `systemone stalled` 無し |

セルフレビュー結果: (1) 網羅性 — 36 セルで `answers == n` かつ停滞ログ無しなら「1 リクエストの N 質問が並列に処理され欠けなく返る」と言える。並列であることの証拠は `TestPoolMaxConcurrency`（同時 4）と `TestConcurrentReadsNotSerialized`（エンジンが絞らない）で、統合は `direct_ms_sum` と `wall_ms` の比較をログに残す。(2) 証拠の十分性 — 停滞テストは経過時間、回答キー集合、ログの各フィールド、ctx cancel の伝播を個別に見る。(3) 迂回の排除 — `TestServiceWithoutPool` により `Run` が直列にフォールバックしないことを固定。統合は health の `workers` で設定が適用されたサーバであることを確認する。(4) 依存関係 — `Pool` → `Service` → `api` の順に単体で固め、統合は実 GGUF のみに依存する。

### 総合判定（testing-rules §12）

全テスト完了後、次のチェックを行い、結果を本計画の末尾「総合判定結果」に記録する。
1. 事実上スキップされたテスト（`t.Skip` 不使用。条件分岐で回避していないか）。
2. テストログ内の `ERROR` / `WARN` / `panic`（API ログの `systemone stalled` WARN を含む）。
3. 迂回による偽成功（`Run` が直列に落ちていないか。`TestServiceWithoutPool` と `TestPoolMaxConcurrency` で固定）。
4. アダプタ・コンフィグの誤適用（Batch / Load が `workers` を書いた設定で 18199 に起動し、`/health` の `workers` が一致するか）。
5. テスト間の順序依存（`-count=1`、各テストが自前のポートとプロセスを持つ）。
6. カバレッジ（新規: `pool.go`、`Run`、`TakeQuestions`、`Text`、`load` 集計、フラグ）。
7. 外部システムの状態（GGUF、`llama-server.exe`、VRAM。スロット 3, 5, 6 の起動が成功したか）。

## Documentation

`prompts/specifications` にこの機能の現行仕様は無い。`prompts/phases/000-foundation/branches/main/ideas/003-DecisionWorkerPool.md` が源である。000 要件 7・8 と 001 の「1 問ずつ」、002 の `llama_parallel` は本計画で置き換わるが、002 の判定と同じ慣例で旧仕様の本文は書き換えない。

#### [MODIFY] [README.md](file://README.md)
*   **更新内容**:
    *   設定表: `llama_parallel` の行を削除し、`workers` (`16`, "Worker goroutines that answer questions; also the cap on concurrent llama calls") と `stall_timeout_ms` (`15000`, "Per-request stall cutoff; missing answers are omitted from a 200 response") を追加。
    *   "Run > 1. Start llama-server" の注記: `--parallel` は `workers` と一致させる必要はない（ワーカーがスロット数を超えた分は llama-server 側で待つ）。
    *   "Decide" のフィクスチャ一覧に `bank.json`（30 questions, choice/score/noul interleaved）と `playground.json`（Jev Playground example）を追加。
    *   "Load" 節: `--questions N`（先頭 N 質問）と `--workers M`（ラベル）を追加し、質問数 × スロット数の例コマンドと、集計キー `questions`, `answers`, `answers_missing`, `questions_per_sec` を説明。`--slots` / `--workers` がラベルである旨。`direct_ms` がワーカー数 > スロット数のとき llama 側の待ちを含むこと。
    *   新節 "Partial answers": ハンドラ単位の停滞判定（`stall_timeout_ms`）で欠けた ID は `answers` から抜け、HTTP 200 のまま WARN `systemone stalled` が出ること。HTTP 同時数が大きく FIFO の待ちが長い場合にも起きること。
    *   "Tests" 節: Batch ベンチマークのコマンドを追加。
