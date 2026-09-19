# 002-DecisionLoad

> **Source Specification**: `prompts/phases/000-foundation/branches/main/ideas/002-DecisionLoad.md`

## Goal Description

`decision-test load` で同じリクエストを goroutine 並列に投げ、concurrency 1、10、50、100 を計測する。llama-server のスロットは 1、2、4 を起動し直し、API から llama へ同時に出してよい件数をそのスロット数に合わせる。既定の起動はスロット 1 のままにする。

## User Review Required

None. スロット数は 1、2、4。`-c 2048` は分けて使う（1 スロットあたり 2048、1024、512）。8 以上は測らない。

## Requirement Traceability

| Requirement (from Spec) | Implementation Point (Section/File) |
| :--- | :--- |
| 1. 変更範囲。既定スロット 1。`/v1/systemone` と `decide` は変えない。429 を足さない。`-c` と `--kv-unified` は変えない | `scripts/setup/run_llama_server.sh`、`internal/config/config.go`、`internal/engine/llamacpp.go` |
| 2. `load` のフラグと、リクエスト数 = concurrency | `cmd/decision-test/main.go`、`internal/cli/load.go` |
| 3. 一斉開始、失敗しても他を止めない、Transport の接続数、10 分タイムアウト、レイテンシと wall_ms | `internal/cli/load.go` |
| 4. 集計 JSON、パーセンタイル `ceil(p/100*n)-1`、終了コード、`direct_ms_sum` | `internal/cli/load.go` |
| 5. `--parallel`、`llama_parallel` セマフォ、組 1/2/4、専用ポート 18280 と 18199 | `run_llama_server.sh`、`llamacpp.go`、`tests/decision_systemone_test.go` |
| 6. 12 回すべて 200。比例は合格線にしない。文脈超過はスキップしない | `TestDecisionSystemOne_Load` |
| 7. 各スロットの concurrency 50 の最中に health が 3 秒以内に 200 | 同じテスト |
| 任意（スロット 8 以上、`-c` 変更、GPU メトリクス、generation 負荷） | 実装しない |

## Proposed Changes

### engine

#### [MODIFY] `features/decision-test/internal/engine/llamacpp_test.go`

*   **Description**: セマフォの容量を、遅い httptest で先に固定する。
*   **Logic**:
    *   `NewClient` の第 3 引数を `parallel int` にする。1 未満は 1 にするのは config 側。ここは渡された正の数を容量にする。
    *   ハンドラは処理中件数の最大を数えて 200ms 寝る。`parallel` 1 で 4 本同時に `ReadLabelLogprobs` しても最大同時数は 1。`parallel` 2 なら最大同時数は 2。
    *   `Health` はセマフォを取らない。推論が塞いでいるあいだに `Health` が 100ms 以内に返る。

#### [MODIFY] `features/decision-test/internal/engine/llamacpp.go`

*   **Technical Design**:

```go
type Client struct {
    baseURL string
    http    *http.Client
    sem     chan struct{}
    log     *logger.Logger
}

func NewClient(baseURL string, log *logger.Logger, parallel int) *Client

func (c *Client) acquire(ctx context.Context) error
func (c *Client) release()
```

*   **Logic**:
    *   `sem` の容量は `parallel`。`acquire` は `sem` へ送るか、`ctx.Done` で戻る。`release` は 1 件受け取る。
    *   `ReadLabelLogprobs`、`Generate`、`Warmup` は処理の前後で acquire / release する。今の `mu.Lock` は削除する。
    *   `Health` と `post` 自体はロックしない。`post` を推論以外から呼ぶ `ResolveLabels` は起動時だけなので、セマフォの外でよい。

### config

#### [MODIFY] `features/decision-test/internal/config/config.go`

*   **Technical Design**:

```go
LlamaParallel int `yaml:"llama_parallel"`
```

*   **Logic**: フィールドが 0 のときは 1。負なら `llama_parallel must be >= 1`。`cmd/decision-test` は `NewClient(..., cfg.LlamaParallel)` を渡す。

#### [MODIFY] `settings/decision-test.yaml`

*   **Logic**: `llama_parallel: 1` を足す。既存のポート値は、作業ツリーにある利用者の変更をこのコミットに含めない。コミット済みの 18080 を基準に、追加行だけを入れる。

### llama 起動

#### [MODIFY] `scripts/setup/run_llama_server.sh`

*   **Logic**:
    *   `--parallel N` を受ける。省略時は 1。`N` が整数かつ 1 以上でなければ stderr に理由を書いて終了 1。
    *   起動引数の `-np` だけをその N にする。`-c 2048 -b 512 -ngl 99 --jinja` は残す。`--kv-unified` は付けない。
    *   `--help` に `--parallel N` を書く。不明な引数は今どおりエラー。

### cli

#### [NEW] `features/decision-test/internal/cli/load.go`

*   **Technical Design**:

```go
type LoadOptions struct {
    Input       string
    Server      string
    Method      string
    Concurrency int
    Slots       int
    JSON        bool
}

type LoadReport struct {
    Slots         int                `json:"slots"`
    Concurrency   int                `json:"concurrency"`
    Requests      int                `json:"requests"`
    Success       int                `json:"success"`
    Errors        int                `json:"errors"`
    Statuses      map[string]int     `json:"statuses"`
    WallMs        float64            `json:"wall_ms"`
    LatencyMs     LatencyMs          `json:"latency_ms"`
    ThroughputRps float64            `json:"throughput_rps"`
    DirectMsSum   *float64           `json:"direct_ms_sum,omitempty"`
}

func Load(ctx context.Context, opt LoadOptions, stdout, stderr io.Writer) error
```

*   **Logic**:
    *   `Requests` は `Concurrency`。goroutine をその本数だけ作り、開始チャネルを閉じるまで待たせてから一斉に POST する。`wall_ms` はその閉鎖時刻から最後の goroutine が終わるまで。
    *   各レイテンシは POST 開始から本文を読み終わるまで。失敗しても他は止めない。
    *   Transport は `MaxConnsPerHost: 0`、`MaxIdleConnsPerHost: Concurrency`。クライアントタイムアウトは 10 分。
    *   成功は状態 200 以上 300 未満。接続エラーの status キーは `"0"`。
    *   パーセンタイルの index は `ceil(p/100*n) - 1`。n が 1 なら 0。補間しない。
    *   `throughput_rps = success / (wall_ms / 1000)`。`wall_ms` が 0 なら 0。
    *   成功本文の `answers` を歩き、`timings.direct_ms` があれば合計して `direct_ms_sum`。1 件も無ければフィールドを出さない。
    *   `--json` のときはこの構造体だけを書く。無いときは `slots`、`concurrency`、`requests`、`success`、`errors`、`status_<code>`、`wall_ms`、`latency_min_ms`、`latency_p50_ms`、`latency_p95_ms`、`latency_max_ms`、`throughput_rps`、あれば `direct_ms_sum`。
    *   失敗が 1 件でも error を返す。集計は先に stdout へ書く。
    *   本文の組み立ては `decide` と同じ `domain.ApplyMethod`。

#### [MODIFY] `features/decision-test/internal/cli/load_test.go`

*   **Logic**: concurrency 10 で httptest の最大同時数が 10。成功 10。`--slots` 相当の `Slots: 2` で JSON の `slots` が 2。状態 500 を 1 件混ぜると `errors` が 1 で `Load` が error を返し、残りの成功は `success` に残る。

#### [MODIFY] `features/decision-test/cmd/decision-test/main.go`

*   **Logic**: `load` サブコマンド。フラグは仕様の 7 つ。`--concurrency` と `--slots` が 1 未満なら実行前にエラー。`--server` の省略時は `decide` と同じ URL 組み立て。

### integration runner

#### [MODIFY] `scripts/process/integration_test.sh`

*   **Logic**: `go test` の引数に `-timeout` `45m` を足す。ヘルプに、負荷試験が Go の既定 10 分を超えるため、と 1 行書く。

### integration test

#### [MODIFY] `tests/decision_systemone_test.go`

*   **Logic**:
    *   `TestDecisionSystemOne_Load` はスロット 1、2、4 の順。各回、llama-server を `-np N` で `127.0.0.1:18280` に起動し、`/health` が 200 になるまで待つ。API は `llama_parallel: N`、`llama_url: http://127.0.0.1:18280`、`api_port: 18199`。
    *   バイナリとモデルのパスは `settings/decision-test.yaml` から読む。ファイルが無ければ `t.Fatal`。`t.Skip` は使わない。
    *   各 N で concurrency 1、10、50、100。`load` はテスト内の HTTP クライアントで同じ集計を行うか、ビルド済み `decision-test` の `load` を実行する。成功数、失敗 0、状態 200、`slots == N`、`wall_ms > 0`。
    *   concurrency 50 の開始後、別 goroutine で 3 秒以内に `GET /health` が 200 かつ `ready == true`。
    *   12 回の数値を `t.Logf` する。比例は見ない。429 と 503 は失敗。
    *   プロセスはテスト終了時に kill する。利用者の 18080 / 18081 は触らない。
    *   既存の DirectAccount は、設定の llama URL（利用者が起動したサーバ）のまま。`llama_parallel` が 1 なら今までどおり 1 本。

## Step-by-Step Implementation Guide

1. **[ ] Semaphore tests, then client**:
    *   Edit `features/decision-test/internal/engine/llamacpp_test.go` for capacities 1 and 2, and health during a held slot.
    *   Edit `features/decision-test/internal/engine/llamacpp.go` and `NewClient` call sites. Update `features/decision-test/internal/config/config.go` so 0 means 1 and negative fails.
2. **[ ] Launch flag**:
    *   Edit `scripts/setup/run_llama_server.sh` to accept `--parallel`.
    *   Add `llama_parallel: 1` to the committed `settings/decision-test.yaml` without taking the local port edit.
3. **[ ] Load CLI tests, then command**:
    *   Add `features/decision-test/internal/cli/load_test.go`, then `load.go`, then the cobra command in `features/decision-test/cmd/decision-test/main.go`.
4. **[ ] Integration**:
    *   Add `-timeout 45m` to `scripts/process/integration_test.sh`.
    *   Add `TestDecisionSystemOne_Load` to `tests/decision_systemone_test.go`.
5. **[ ] Verification Plan**:
    *   Run the commands below. Do not finish while they fail.
    *   Write the section 12 verdict into this file after the runs.

## Verification Plan

### Automated Verification

1.  **Build & Unit Tests**:

```bash
./scripts/process/build.sh
```

2.  **Integration Tests**:

```bash
./scripts/process/integration_test.sh --specify "TestDecisionSystemOne_Load|TestDecisionSystemOne_DirectAccount"
```

    *   **Log Verification**: 12 行の計測ログに `slots` が 1、2、4 のいずれか、`concurrency` が 1、10、50、100。`level=ERROR` が API ログに無いこと。health の 200 が各スロットの 50 件中に出ること。

このスクリプトに `--categories` は無い。全カテゴリ一括は完了条件にしない。

### E2E Tests

GUI の E2E は作らない。計測対象はローカル HTTP と CLI である。実 llama-server と実 GGUF の 12 回は `tests/decision_systemone_test.go` が担う。手動の `load` 実行は代替にしない。

#### [MODIFY] `tests/decision_systemone_test.go`

*   **テストケース**: `TestDecisionSystemOne_Load`。既存の DirectAccount は残す。
*   **検証ポイント**: 12 回がすべて HTTP 200。セマフォ容量の単体テストが 1 と 2 を区別している。health が 50 件の最中に返る。スループットの比例は見ない。

### テスト項目のセルフレビュー

1.  **網羅性**: クライアント並列、集計、セマフォ容量、起動フラグ、12 回の受理、health、choice 退行が通れば、仕様の完了条件を言える。言えないのは GPU 使用率とスロット 8 以上だが、仕様が要求していない。
2.  **証拠**: 200 だけでなく同時数、`slots`、失敗時の終了コード、health の 3 秒を見る。
3.  **迂回**: セマフォが常に 1 だと容量 2 のテストが落ちる。`--parallel` を無視すると統合のスロット 4 で文脈または待ち時間が 1 と区別できないが、合否は 200 なので、起動引数に `-np` がその N であることをテストがプロセス生成時に固定する。
4.  **依存**: セマフォと load の単体が通ってから統合に進む。

### 総合判定

全テスト完了後、testing-rules の §12.2 を実ログに対して確認し、この節に判定を書く。

## Documentation

`prompts/specifications` にこの機能の現行仕様は無い。`prompts/phases/000-foundation/branches/main/ideas/002-DecisionLoad.md` が源である。`000-DecisionTest` の「ミューテックスで直列化」は、`llama_parallel` が 1 のときの動作として残る。000 は書き換えない。
