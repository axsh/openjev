# 001-ScoreNoul

> **Source Specification**: `prompts/phases/000-foundation/branches/main/ideas/001-ScoreNoul.md`

## Goal Description

`features/decision-test` の `POST /v1/systemone` に `type: score` と `type: noul` を足す。推論エンジンのメソッドは増やさない。既存の A…T readout と生成検証を、型ごとのラベル割り当てと集計に差し替える。`type: choice` の成功パスは変えない。

## User Review Required

None. ユーザーがこの計画の直後に実装実行を指示している。仕様 001 の式、ラベル固定、noul から confidence を外す決定は、そのまま実装する。

## Requirement Traceability

| Requirement (from Spec) | Implementation Point (Section/File) |
| :--- | :--- |
| 1. 変更先は `features/decision-test` のみ。choice の成功パス、エンドポイント、CLI フラグ、設定、llama-server 起動は変えない。255 選択肢、SSE、プロセス内バインディング、Ollama / LM Studio はやらない | Proposed Changes の範囲外。新しいエンドポイントもフラグも作らない |
| 2. 混在、出現順、質問 ID をモデルに渡さない、method はリクエスト全体、direct / generation / both、欠落ラベル 502、`elapsedMs` はハンドラ、confidence 式は choice と同じ | `internal/decision/service.go`、`internal/api/api.go`（ElapsedMs は既存のまま） |
| 3. score の criteria は 2〜10 の文字列配列。空、空白、重複、オブジェクト、長さ 1 と 11 は 422。本文は要素そのもの | `internal/domain/validate.go` |
| 4. `score = Σ i * p_i`。丸めない。legend は `"0"` から。choice キーは出さない。同率でも argmax しない | `internal/decision/math.go` の `WeightedScore`、`internal/decision/service.go` |
| 5. noul の criteria は省略または null 可。付けるなら `true` と `false` だけ。ラベルは true=A、false=B。キー順に依存しない | `internal/domain/validate.go` |
| 6. `noul` は P(true)。choice / probabilities / confidence / score / legend は出さない。0.5 で切らない | `internal/decision/service.go`、`internal/domain/types.go` |
| 7. プロンプト文言は維持。score は `Levels are ordered from lowest to highest.`、noul は `A is yes. B is no.`。生成キーは `"<label>: <本文>"`。既存 GBNF は `A. ` 行を型で分けない | `internal/prompt/prompt.go`、`internal/engine/llamacpp.go`（文法関数は変更しない。テストで score 行がキーになることを固定する） |
| 8. generation は choice を維持。score は `generation.score` と段階キー。noul は `generation.noul` と `true`/`false`。`generation.choice` は出さない。valid false では決定値も probabilities も付けない | `internal/decision/service.go`、`internal/domain/types.go` |
| 9. CLI フラグは増やさない。人間可読は `score:` と `noul:`。noul に confidence 行は出さない | `internal/cli/decide.go` |
| 検証シナリオ 1〜7 | Verification Plan。422 は単体。1〜5 と 7 は `tests/decision_systemone_test.go` |
| 任意要件（丸め、しきい値、noul confidence、instructions の object/array 受理、11 段階以上） | 実装しない。object/array の instructions は 422 のまま |

## Proposed Changes

ボトムアップで、テストを先に書く。

### domain

#### [MODIFY] `features/decision-test/internal/domain/validate_test.go`

*   **Description**: score / noul の受理と 422 を先に落とす。既存の `type_score` と `type_noul` が `not supported` を期待している箇所は、受理に書き換える。
*   **Logic**:
    *   受理: `criteria` が `type` より前でも score の配列を読める。段階キーは `"0"`,`"1"`,`"2"`。`Text` は `Calm` のように要素そのもの（`0: Calm` にはしない）。
    *   受理: noul の JSON が `false` キーを先に書いても、`Criteria[0].Key` は `true`、`Criteria[1].Key` は `false`。本文は説明文字列。`true: ...` 連結はしない。
    *   受理: criteria 省略と JSON `null` は、本文 `true` / `false` の 2 件。
    *   422: `features/decision-test/testdata/score-one.json`、`score-eleven.json`、`score-object.json`、`score-empty.json`、`noul-true-only.json`、`noul-extra-key.json`、`type-rank.json`。
    *   422: 空白だけの score 要素、同じ文字列の重複、noul の空文字、noul の配列。
    *   `ApplyMethod` が score の配列と noul の説明文を壊さず、`options.method` だけを上書きする。

#### [MODIFY] `features/decision-test/internal/domain/types.go`

*   **Description**: レスポンスから、型に関係ないキーを消せるようにする。
*   **Technical Design**:

```go
type Answer struct {
    Type          string             `json:"type"`
    Choice        string             `json:"choice,omitempty"`
    Score         *float64           `json:"score,omitempty"`
    Noul          *float64           `json:"noul,omitempty"`
    Legend        map[string]string  `json:"legend,omitempty"`
    Probabilities map[string]float64 `json:"probabilities,omitempty"`
    Confidence    *float64           `json:"confidence,omitempty" doc:"1 - H(p) / ln(n). H is the natural-log entropy of the option distribution. 1 when peaked, 0 when uniform. Not Jev's unpublished definition."`
    Method        Method             `json:"method"`
    Generation    *Generation        `json:"generation,omitempty"`
    Timings       *Timings           `json:"timings,omitempty"`
}

type Generation struct {
    Choice          string             `json:"-"`
    EmitChoice      bool               `json:"-"`
    Score           *float64           `json:"score,omitempty"`
    Noul            *float64           `json:"noul,omitempty"`
    Probabilities   map[string]float64 `json:"probabilities,omitempty"`
    Valid           bool               `json:"valid"`
    ValidationError string             `json:"validation_error"`
    GeneratedText   string             `json:"generated_text"`
    TTFTMs          float64            `json:"ttft_ms"`
    TotalMs         float64            `json:"total_ms"`
    OutputTokens    int                `json:"output_tokens"`
}
```

*   **Logic**:
    *   `Generation.MarshalJSON` は choice 型のときだけ `choice` を出す。無効な生成でも choice 型なら空文字キーを残す（今の choice JSON を変えない）。score と noul では `EmitChoice` を false にし、キー自体を出さない。
    *   `Answer.Choice` の `omitempty` により、空文字の choice キーは消える。choice の成功パスは非空の key を入れるので、今まで通りキーが出る。

#### [MODIFY] `features/decision-test/internal/domain/validate.go`

*   **Description**: `type` を見てから criteria を解釈する。
*   **Logic**:
    *   `parseQuestion` は `criteria` を `json.RawMessage` のまま持つ。`type` より前に来ても、オブジェクトを閉じたあと `Validate` が解釈する。
    *   `instructions` が文字列でないときは `instructions must be a string`。`strings.TrimSpace` が空なら、今と同じ `instructions must not be empty`。
    *   `type == "choice"`: 今のオブジェクト解釈。長さ 2〜20。`Text` は description が nil なら key、否则 `key + ": " + description`。配列なら `criteria must be an object`。
    *   `type == "score"`: 配列でなければ `score criteria must be an array`。長さが 2 未満または 10 超なら `score requires 2 to 10 levels`。要素が文字列でなければ 422。`strings.TrimSpace` が空なら `score level must not be blank`。完全一致の重複は `duplicate score level`。`Criterion.Key` は `strconv.Itoa(i)`（0 始まり）。`Text` は要素そのもの。`Description` は nil。
    *   `type == "noul"`: raw が無い、または JSON `null` なら `[{Key:true, Text:true}, {Key:false, Text:false}]`。配列なら `noul criteria must be an object`。オブジェクトのキーは `true` と `false` の両方だけ。欠ければ `noul criteria requires true and false`。第三のキーは `noul criteria has unexpected key`。値が空または空白だけなら `noul criteria must not be blank`。スライス順は必ず true が先、false が後。JSON のキー順は見ない。`Text` は値の文字列そのもの。
    *   それ以外の type は、criteria の形を見る前に `question type %q is not supported`。
    *   `marshalRequest` は choice を今のオブジェクト、score を文字列配列、noul を `{"true": Text, "false": Text}` で書き戻す。CLI の `--method` が説明文を落とさないため。

### decision

#### [MODIFY] `features/decision-test/internal/decision/math_test.go`

*   **Description**: モデルを呼ばずに加重平均を固定する。
*   **Logic**: `[0.1, 0.2, 0.7]` は 1.6。一様な 3 段階は 1。`[0, 0, 1]` は 2。誤差は 1e-9。

#### [MODIFY] `features/decision-test/internal/decision/math.go`

*   **Technical Design**:

```go
func WeightedScore(probs []float64) float64 {
    score := 0.0
    for i, p := range probs {
        score += float64(i) * p
    }
    return score
}
```

*   **Logic**: 最近傍の段階へ丸めない。argmax は呼ばない。noul 用の関数は作らない。P(true) はラベル順の `probs[0]`（A）をそのまま使う。

#### [MODIFY] `features/decision-test/internal/decision/generate_test.go`

*   **Description**: score と noul の生成キーを既存の検証に足す。
*   **Logic**: criteria の `Text` が `Calm` なら期待キーは `A: Calm`。noul の省略本文なら `A: true`、`B: false`。過不足、0〜1 の外、合計が `1±0.02` の外は `valid: false`。`0.51+0.51` は通る（既存の `0.02+1e-9`）。markdown フェンスは成功にしない。`ChoiceKey` は choice 用に残してよいが、score / noul のサービスはそれを採用しない。

#### [MODIFY] `features/decision-test/internal/decision/service_test.go`

*   **Description**: 集計と、valid false の欠落を固定する。ログに `question_type` が出る。
*   **Logic**:
    *   score direct: 分布を与え、`score` が `WeightedScore` と一致。`legend["0"]` が本文。`Choice` は空。`Confidence` は 0 以上 1 以下。
    *   noul direct: A の logit を高くし、`Noul` が softmax 後の `probs[0]`。`Probabilities` と `Confidence` と `Score` と `Legend` と `Choice` は空。
    *   固定分布の単体: サービスに渡す logit を直接は作れないので、noul の `0.95` は `PickLogprobs` ではなく、集計関数の入力として `probs := []float64{0.95, 0.05}` を `applyDistribution` に渡して `Noul == 0.95` を見る。JSON に `confidence` が無いことも `json.Marshal` で見る。
    *   generation valid の score: トップレベル `score` と `generation.score` が同じ加重平均。`generation.choice` の JSON キーは無い。
    *   generation valid の noul: トップレベル `noul` は生成 JSON の A 側。`generation.probabilities` のキーは `true` と `false`。トップレベルに `probabilities` は無い。
    *   generation invalid: `score` も `noul` も `probabilities` も `confidence` も JSON に無い。HTTP エラーにはしない（サービスは error nil）。
    *   choice の既存 `TestServiceDirect`、`TestServiceBothOrder`、`TestServiceGenerationInvalid`、`TestServiceMissingLogit` は成功したままにする。

#### [MODIFY] `features/decision-test/internal/decision/service.go`

*   **Description**: `answer.Type` を質問の type にする。集計を型で分ける。
*   **Logic**:
    *   `prompt.Messages` に `question.Type` を渡す。direct と generation の両方。
    *   ラベル数は `len(question.Criteria)`。noul は常に 2。score は配列長。`len(s.Labels)` 未満なら今と同じ `not enough labels`。
    *   direct の確率 `probs` のあと:
        *   choice: 今どおり `ArgMax`、`Confidence`、key ごとの `probabilities`。
        *   score: `score = WeightedScore(probs)`。`confidence = Confidence(probs)`。`legend[key] = text`。`probabilities` のキーは段階番号。`Choice` は空のまま。
        *   noul: `noul = probs[0]`。他の決定フィールドは付けない。`noul >= 0.5` の bool 変換はしない。
    *   generation が valid のとき:
        *   choice: 今どおり `Choice` と `probabilities` と、generation メソッドなら confidence。`EmitChoice = true`。
        *   score: 順序配列から `WeightedScore`。`generation.Score` と段階キーの `generation.Probabilities`。`EmitChoice = false`。generation メソッドならトップレベルも同じ score、probabilities、confidence。
        *   noul: `generation.Noul = probs[0]`（criteria 順で index 0 が true）。`generation.Probabilities` は `true` / `false`。トップレベルには probabilities を上げない。generation メソッドならトップレベル `noul` だけ。confidence は付けない。
    *   valid false: 決定値をどれもセットしない。`generation.Score`、`Noul`、`Probabilities` は nil。
    *   timings は今どおり。`ratio = generation_ms / direct_ms`。`ElapsedMs` はサービスが埋めない。
    *   ログは `component=decision` のまま。`direct completed` と `generation started` に `question_id` と `question_type` を付ける。choice にも付ける。メッセージ文字列は変えない。

### prompt

#### [MODIFY] `features/decision-test/internal/prompt/prompt_test.go`

*   **Description**: choice の既存文面が 1 文字も変わらないことを先に固定し、そのあと score と noul の 1 文を足す。
*   **Logic**:
    *   シグネチャに `questionType string` を足す。既存呼び出しは `"choice"`。
    *   choice の user 本文は今のゴールデンと完全一致。`Levels are ordered` も `A is yes` も含まれない。
    *   score は `Allowed options:` の直前が `Levels are ordered from lowest to highest.\n`。選択肢行は `A. Calm`。system 文と generation の Route north / Route south 例は変えない。
    *   noul は直前が `A is yes. B is no.\n`。criteria 省略の本文なら `A. true` と `B. false`。期待キーは `A: true`、`B: false`。
    *   質問 ID はプロンプトに出さない。

#### [MODIFY] `features/decision-test/internal/prompt/prompt.go`

*   **Technical Design**:

```go
func Messages(state string, instructions string, criteria []domain.Criterion, mode domain.Method, questionType string) []Message
```

*   **Logic**: choice の連結は今のまま。score と noul だけ、`Allowed options:` の前に上の 1 文を入れる。direct の `Reply with exactly one option letter from: A, B, C.` と generation 指示の全文は変えない。

### engine

#### [MODIFY] `features/decision-test/internal/engine/llamacpp_test.go`

*   **Description**: 生成 GBNF が score の `A. Calm` 行もキーにすることを固定する。`generationGrammar` の実装は変えない。
*   **Logic**: user 本文に `A. Calm`、`B. Frustrated`、`C. Very angry` があるとき、文法文字列にそのキーがこの順で含まれる。`A is yes. B is no.` は `A. ` 行ではないのでキーにしない。

### api

#### [MODIFY] `features/decision-test/internal/api/api_test.go`

*   **Description**: `TestRejectsBeforeEngine` から「score は 422」を外す。シナリオ 6 のファイルは Engine 呼び出し 0 の 422。score と noul の direct は 200 で、choice キーが無く、noul に confidence が無い。
*   **Logic**: カウントエンジンの logit は今どおり。勝ち段階は断言しない。score は `Σ i*p_i` と 1e-6 以内。OpenAPI に `1 - H(p) / ln(n)` が残る。

`internal/api/api.go` は変えない。422 / 502 / ElapsedMs は既存の分岐のまま。

### cli

#### [MODIFY] `features/decision-test/internal/cli/decide_test.go`

*   **Description**: `--json` 無しの score と noul を固定する。フラグは増やさない。
*   **Logic**: score は `score:`、`confidence:`、段階番号の昇順 `0  0.000`。noul は `noul:` だけで、`confidence` 行は無い。`direct_ms` / `generation_ms` / `ratio` の出し分けは choice と同じ。

#### [MODIFY] `features/decision-test/internal/cli/decide.go`

*   **Logic**: 人間可読は `type` を見る。score は `score: %.3f` のあと confidence、そのあと確率。キーは `sort.Strings`（0〜9 なので昇順になる）。noul は `noul: %.3f` のみ。choice は今の `choice:` 行を残す。

### integration

#### [MODIFY] `tests/decision_systemone_test.go`

*   **Description**: 実 llama-server でシナリオ 1〜5 と、choice の退行（既存 4 本）を残す。`t.Skip` は使わない。llama-server が無ければ `t.Fatal`。
*   **テストケース**:
    *   `TestDecisionSystemOne_Score`: ポート 18195。`features/decision-test/testdata/score.json` を direct。HTTP 200。`type` は `score`。`choice` キーは無い。legend は Calm / Frustrated / Very angry。確率キーは `0`,`1`,`2` だけ。合計 1±1e-6。score は加重平均と 1e-6。範囲は 0 以上 2 以下。confidence は 0 以上 1 以下。`output_tokens` は 1。`generation` は無い。`direct_ms` は 0 より大きい。同じサーバで `--method` 相当の generation を POST し、valid ならキー順が `A: Calm`、`B: Frustrated`、`C: Very angry`。合計 1±0.02。トップレベル `score` と `generation.score` が加重平均と 1e-6。`generation.choice` は無い。`ttft_ms` と `total_ms` は 0 より大きく、`ttft_ms` は `total_ms` 以下。どの段階が最大かは断言しない。
    *   `TestDecisionSystemOne_Noul`: ポート 18196。`noul.json`。決定フィールドは `type`、`noul`、`method`、`timings` だけ。`noul` は 0 以上 1 以下。bool フィールドは無い。`output_tokens` は 1。
    *   `TestDecisionSystemOne_NoulDefaultCriteria`: ポート 18197。`noul-omit.json` の direct は `noul` が 0 以上 1 以下。続けて generation。valid ならキー順 `A: true`、`B: false`。合計 1±0.02。トップレベル `noul` は A 側と 1e-6。
    *   `TestDecisionSystemOne_Mixed`: ポート 18198。`mixed.json` を both。3 ID が返る。choice は確率和 1±1e-6、choice は 3 key のいずれか、generation がある。score と noul は上の形。各 `ratio` は `generation_ms / direct_ms` と相対誤差 1e-6。ログは質問ごとに `direct completed` のあと `generation started`。`question_id` は `queue`、`frustration`、`is_urgent` の順。`question_type` は `choice`、`score`、`noul`。`level=ERROR` は無い。`output_tokens` は 3 以上。
*   **Logic**: 既存の `withMethod` は、すでに `options` があるファイルでは使わない。method を変えるときは、末尾の `}` の前に足すとキーが重複する。generation の上書きは、入力を `domain.ApplyMethod` と同じ結果になる JSON にするか、`options` を置換してから POST する。`tests` モジュールは `openjev/tests` で domain を import しない。テスト内で `options` を置換する小さな関数を置く。

## Step-by-Step Implementation Guide

1. **[ ] Domain tests, then parser**:
    *   Edit `features/decision-test/internal/domain/validate_test.go` with the accept and 422 cases above.
    *   Edit `features/decision-test/internal/domain/types.go` and `features/decision-test/internal/domain/validate.go` until those tests pass.
    *   `Generation.MarshalJSON` emits `choice` only when `EmitChoice` is true.
2. **[ ] Weighted score**:
    *   Edit `features/decision-test/internal/decision/math_test.go`, then add `WeightedScore` to `features/decision-test/internal/decision/math.go`.
3. **[ ] Prompt sentence**:
    *   Edit `features/decision-test/internal/prompt/prompt_test.go`, then add the `questionType` argument and the two sentences in `features/decision-test/internal/prompt/prompt.go`.
    *   Update every `Messages` call site in the same step so the package builds.
4. **[ ] Aggregation**:
    *   Edit `features/decision-test/internal/decision/generate_test.go` and `features/decision-test/internal/decision/service_test.go`.
    *   Edit `features/decision-test/internal/decision/service.go` so direct, generation, and both fill score and noul as specified. Do not set `ElapsedMs`.
    *   Add the grammar assertion in `features/decision-test/internal/engine/llamacpp_test.go`. Do not change `generationGrammar`.
5. **[ ] API and CLI**:
    *   Rewrite the score 422 expectation in `features/decision-test/internal/api/api_test.go`. Add the fixture 422 cases and the direct score / noul shape checks.
    *   Edit `features/decision-test/internal/cli/decide_test.go`, then `features/decision-test/internal/cli/decide.go`.
6. **[ ] Integration tests**:
    *   Add the four `TestDecisionSystemOne_*` functions to `tests/decision_systemone_test.go` before running them.
7. **[ ] Verification Plan**:
    *   Run the commands in Verification Plan. Do not finish while they fail.
    *   After they pass, write the section 12 verdict into this plan's Verification Plan and mark these boxes.

## Verification Plan

### Automated Verification

1.  **Build & Unit Tests**:

```bash
./scripts/process/build.sh
```

2.  **Integration Tests**:

```bash
./scripts/process/integration_test.sh --specify "TestDecisionSystemOne"
```

    *   **Log Verification**: Mixed のログに `level=DEBUG`、`component=decision`、`msg="direct completed"` のあと `msg="generation started"`。`question_id` は `queue`、`frustration`、`is_urgent` の順。`question_type` は `choice`、`score`、`noul`。`level=ERROR` は無い。

3.  **Narrow rerun**（失敗した追加分だけを見るとき）:

```bash
./scripts/process/integration_test.sh --specify "TestDecisionSystemOne_Score|TestDecisionSystemOne_Noul|TestDecisionSystemOne_Mixed"
```

このスクリプトに `--categories` は無い。全カテゴリ一括は完了条件にしない。

### E2E Tests

GUI の E2E は作らない。この機能は VSCode 拡張ではなく、ローカル HTTP サーバと CLI である。実 llama-server と実 GGUF を使う検証は `tests/decision_systemone_test.go` が担う。手動の `decide` 実行は、このテストの代替にしない。

#### [MODIFY] `tests/decision_systemone_test.go`

*   **テストケース**: `TestDecisionSystemOne_Score`、`TestDecisionSystemOne_Noul`、`TestDecisionSystemOne_NoulDefaultCriteria`、`TestDecisionSystemOne_Mixed`。既存の Health / DirectAccount / GenerationEmail / BothOrder は残す。
*   **検証ポイント**: score が段階の加重平均であること。noul が true 側の確率だけであること。choice の既存統合がまだ通ること。実モデルの値が Jev の校正値と一致することは要求しない。

### テスト項目のセルフレビュー

1.  **網羅性**: 末端の `WeightedScore` とラベル順、入力 422、プロンプト文、集計 JSON、CLI 表示、実モデルの 4 シナリオと choice 退行が揃えば、仕様の完了条件を言える。言えないのは Jev 校正値との一致だが、仕様が要求していない。
2.  **証拠**: 200 だけでなく、加重平均との差、禁止キーの不在、生成キーの順序、ratio の計算、ログ順を見る。
3.  **迂回**: score を argmax にすると加重平均テストが落ちる。noul をキー順で A/B すると `false` 先のテストが落ちる。confidence を noul に付けると JSON テストが落ちる。
4.  **依存**: 単体（math、domain、prompt、service）が通ってから統合に進む。統合は単体が証明した式を、実トークン上で再確認する。

観点: 正常系（score 3 段階、noul ありなし、混在、generation、both）、異常系（シナリオ 6 と空白・重複）、外部連携（統合の llama `/health` と GGUF）、一貫性（生成分布から同じ score / noul を再計算）、状態（valid false は 200 のまま決定値なし）、設定（method はリクエスト全体。CLI `--method` が配列を壊さない）、副作用（Engine 呼び出し 0、ERROR ログなし）。

### 総合判定

全テスト完了後、testing-rules の §12.2 を実ログに対して確認し、この節に判定を書く。「全テスト成功」だけでは動作確認完了にしない。

## Documentation

`prompts/specifications` に、この機能の現行仕様書は無い。`prompts/phases/000-foundation/branches/main/ideas/000-DecisionTest.md` の「score と noul は 422」は、当時の choice 仕様として残す。置き換えは `prompts/phases/000-foundation/branches/main/ideas/001-ScoreNoul.md` が既に書いている。この計画では 000 を書き換えない。
