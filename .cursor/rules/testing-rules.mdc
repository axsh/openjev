---
description: Testing Rules
alwaysApply: false
---

# テストのルール (Testing Rules)

本プロジェクトでは、テストの実行方法はシェルスクリプト `scripts/` に集約されています。

## 1. テスト実行の標準手順 (Test Execution Matrix)

コンポーネントとテストレベルに応じて、適切なスクリプトを使用してください。


| Component          | Test Level      | Command                                                | Purpose                      |
| ------------------ | --------------- | ------------------------------------------------------ | ---------------------------- |
| **Backend (Go)**   | **Unit**        | `scripts/process/build.sh --skip-frontend --skip-etc`  | ロジックの正当性確認。Fail Fast。        |
| **Backend (Go)**   | **Integration** | `scripts/process/integration_test.sh`                  | コンテナやAPIとの連携確認。              |
| **Frontend (GUI)** | **Unit**        | `scripts/process/build.sh`                             | Webviewコンポーネント等の単体テスト。       |
| **Frontend (GUI)** | **E2E**         | `scripts/process/integration_test.sh --categories gui` | VSCode上での挙動検証 (Test Driver)。 |
| **Full Stack**     | **Pipeline**    | `.agent/workflows/build-pipeline.md`                   | PR/コミット前の全体健全性確認。            |


### Linux / Remote-SSH での全体ビルド (`build.sh`)

ワークスペースの OS が **Linux** のとき、および **Cursor / VS Code の Remote-SSH でリモートが Linux** のとき、`scripts/process/build.sh` を実行する場合は **`--skip-etc`** を付けること（例: `./scripts/process/build.sh --skip-etc`）。`etc` 配下（`mcp-command-runner` / `image-inspector` 等）は Windows 向け前提やサンドボックス制約で失敗しやすいため。

### Linux / Remote-SSH での統合テスト (`integration_test.sh`) — headless

同じく **Linux** または **Remote-SSH のリモートが Linux** のとき:

1. **`--headed` と `--ui` を付けない**（拡張 GUI E2E は Playwright の **headless 既定**のままにする）。
2. **`xvfb-run -a` で必ずラップ**してから `integration_test.sh` を実行する（**`./scripts/process/integration_test.sh` を直接実行しない**。`--categories` の有無や `--resume` / `--specify` の有無にかかわらず同様）。例: `xvfb-run -a ./scripts/process/integration_test.sh`、`xvfb-run -a ./scripts/process/integration_test.sh --categories llm`。根拠は `features/frontend/scripts/integration_test.sh` 先頭コメント。**`xvfb-run` が無い場合は Xvfb 系パッケージを導入してから実行**する。

## 2. 実行順序のルール (Execution Order Rule)

> [!CRITICAL]
> **Always Build Before Integration Test**
>
> 統合テスト (`scripts/process/integration_test.sh`) を実行する前には、**必ず** ビルド (`scripts/process/build.sh`) を成功させてください。
>
> - 理由: 統合テストはビルド済みのバイナリ（拡張機能 `*.vsix` やサーバーバイナリ）や、コンパイル済みのWebviewアセットを使用します。ビルドをスキップしてソースコードだけ修正しても、テスト対象のバイナリは古いままとなり、修正が反映されません。
> - 手順（ビルド成功後に統合テスト）:
>   ```bash
>   # Linux、または Remote-SSH のリモートが Linux のとき
>   # build: --skip-etc。integration: --headed / --ui 禁止。xvfb-run -a は常に付ける（直接 integration_test.sh を叩かない）。xvfb-run が無い場合は Xvfb 系パッケージを導入する。
>   ./scripts/process/build.sh --skip-etc && xvfb-run -a ./scripts/process/integration_test.sh ...
>   ./scripts/process/build.sh --skip-etc && xvfb-run -a ./scripts/process/integration_test.sh --categories llm ...
>   # それ以外（例: macOS）— 検証方針に合わせてフラグを選ぶ
>   ./scripts/process/build.sh && ./scripts/process/integration_test.sh ...
>   ```

## 3. エラー修正フロー

テストエラーが発生した場合、以下のフローで修正を行ってください。

**Linux** または **Remote-SSH のリモートが Linux** のときは、この節および直下の **Tips** にある **`./scripts/process/integration_test.sh` を必ず `xvfb-run -a` でラップ**すること（直接実行しない）。

1. **Fail Fast**: `scripts/process/build.sh` が失敗した場合、直ちに修正し再実行する。
2. **Log Analysis**: バックエンドサーバー（Go）の問題解決には、`syslogd` コンテナのログを確認することが重要です。
  - `scripts/setup/setup_containers.sh` により準備された環境で、以下のツールを使用してログを確認します。
  - フィードバックを得る手段として積極的に活用してください。
  ```bash
  # 直近のログを確認
  ./scripts/utils/view-syslog.sh --tail 100

  # ログを監視しながらテスト実行（別ターミナルで実行推奨）
  ./scripts/utils/view-syslog.sh -f
  ```
3. **Filter Execution**: 特定の統合テストのみ失敗した場合、`--specify` オプションを使用して**該当テストのみ**を再実行する。
  ```bash
  # Linux / Remote-SSH（リモートが Linux）
  xvfb-run -a ./scripts/process/integration_test.sh --specify "TestAuthentication"
  # それ以外（例: macOS）
  ./scripts/process/integration_test.sh --specify "TestAuthentication"
  ```
  これにより、全テスト実行（数分）を待つことなく高速にデバッグが可能。
4. **Full Verification**: 修正後の確認ができたら、最後にオプションなしでスクリプトを実行し、リグレッションがないか確認する（**Linux / Remote-SSH** では `xvfb-run -a ./scripts/process/integration_test.sh`）。

### 効率的なテスト実行 (Tips)

**Linux / Remote-SSH（リモートが Linux）** のときは、下記の `./scripts/process/integration_test.sh` を **`xvfb-run -a` でラップ**してから実行すること（§1 および上記セクション 3 と同じ）。

- **GUIテストのみ実行**: VSCode拡張機能E2Eテストのみを行う場合。
  ```bash
  # Linux / Remote-SSH
  xvfb-run -a ./scripts/process/integration_test.sh --categories gui
  # それ以外（例: macOS）
  ./scripts/process/integration_test.sh --categories gui
  ```
- **バックエンド統合テストのみ実行**: 特定のカテゴリ（例: llm）を指定する場合。
  ```bash
  # Linux / Remote-SSH
  xvfb-run -a ./scripts/process/integration_test.sh --categories llm
  # それ以外（例: macOS）
  ./scripts/process/integration_test.sh --categories llm
  ```

## 4. 統合テストの構成と命名

> [!WARNING]
> **実行コマンドの制限**:
> `go test`, `npm test` コマンドを直接実行しないでください（特定ディレクトリでの開発作業を除く）。
> 検証やPR前の確認では、必ず `scripts/process/integration_test.sh` または `scripts/process/build.sh` を使用してください。

統合テストは `tests/integration/` (または `tests/`) 配下に配置し、ファイル名でカテゴリを識別します。

- **命名規則**: `{カテゴリ}_{機能名}_test.go`
  - 例: `llm_adapter_test.go`, `server_huma_test.go`
- **タグ**: ファイル先頭に `// +build integration` (または `//go:build integration`) を記述し、単体テストから除外すること。

## 5. 環境依存性

統合テストは `scripts/setup/setup_containers.sh` によってセットアップされる環境（Dockerコンテナ等）を前提として動作します。
テスト実行前に環境が立ち上がっていない場合、テストが失敗する可能性があります。その場合は `setup_containers.sh` を再実行してください。

## 6. テストスキップの禁止

**テストのスキップは厳格に禁止します。** 以下のルールを遵守してください。

### 6.1. スキップの禁止

- `t.Skip()`, `t.Skipf()`, `t.SkipNow()` の使用は**一切禁止**します。
- 条件分岐でテストを回避することも禁止します。

### 6.2. 必須の対応方法

テストの前提条件が満たされていない場合は、**必ずエラーとして扱う**こと：

**❌ 禁止 (スキップ)**

```go
if !found {
    t.Skip("No Google/Gemini profile found")
}
```

**✅ 推奨 (エラー)**

```go
if !found {
    t.Fatalf("No Google/Gemini profile found in model_profiles.yaml")
}
```

### 6.3. 理由

- **設定の明示**: テストがスキップされると、必要な設定が欠けていることが見逃される可能性があります。
- **CI/CD の健全性**: 全テストが実行可能な状態を維持することで、パイプラインの信頼性が向上します。
- **責任の明確化**: スキップではなくエラーにすることで、問題の解決責任が明確になります。

テストの前提条件（設定ファイル、環境変数、プロファイルなど）が不足している場合は、それらを整備してからテストを実行してください。

## 7. テストのタイムアウトの考え方

テストが時間切れで落ちたとき、**タイムアウト値を延長することは最後の手段**です。
値を延ばす前に、まず「処理そのものが遅いのか」「ハング／デッドロックしていないか」「非同期の待ち方が不適切ではないか」を確認してください。

### 7.1 ケース別の設定方針

ケースごとに想定完了時間が異なるため、タイムアウト値の決め方もそれぞれ異なります。


| #   | ケース                                             | 想定完了時間   | タイムアウト値の目安                                 |
| --- | ----------------------------------------------- | -------- | ------------------------------------------ |
| A   | 成功時はすぐ終わることが期待できるが、**非同期処理のためにタイムアウト設定が必要**なテスト | 数ms〜数百ms | **できるだけ短く** 設定する                           |
| B   | 実行に長時間かかる、もしくは**遅くなるケースがありうる**ことが事前にわかっているテスト   | 数秒〜数十秒   | 原則現状維持。**実際にタイムアウトで打ち切られたと推測できる場合のみ**延長を検討 |
| C   | 上記以外（特に指定がない一般的な処理）                             | 3秒以内     | **3秒以内** を前提にする                            |


### 7.2 各ケースの補足

- **ケース A（即終了を期待する非同期テスト）**
短いタイムアウトを維持することで、「意図せず処理が長時間ブロックされる」不具合を早期に検知できます。安易に延長するとバグを見逃す原因になります。
- **ケース B（長時間／不安定になり得るテスト）**
延長してよいのは、ログ・スタックトレース等から「処理が終わる前にタイムアウトで打ち切られた」ことが合理的に推測できる場合だけです。
単に「時々失敗する」「CIで不安定」という理由だけでの延長は禁止します。まずは原因調査を優先してください。
- **ケース C（それ以外）**
特段の理由がない限り、処理は3秒以内に完了する前提でテストを設計してください。
3秒を超えて完了しない場合は、テスト対象の設計を見直すか、ケース A / B のどちらに該当するかを再評価してください。

## 8. プロセスのクリーンアップ（VSCode E2Eテスト）

VSCode拡張機能のE2Eテスト (`scripts/process/integration_test.sh --categories gui`) が中断された場合、ElectronやPlaywrightのプロセスがバックグラウンドに残存し、次回のテスト実行やVSCodeの挙動に悪影響を与えることがあります。
テストが正常に終了しなかった場合は、以下のコマンドで残存プロセスを強制終了してください。

```bash
pkill -f "integration_test.sh" && pkill -f playwright && pkill -f "Code Helper"
```

## 9. VSCode E2Eテスト実装ルール

VSCode拡張機能のE2Eテストは、以下の設計書及びルールに従って実装してください。

**詳細設計・実装ガイドライン**: `[prompts/specifications/gui-e2e-testing-design.md](../specifications/gui-e2e-testing-design.md)`

### 9.1 基本原則

1. **Test Driver Patternの強制**:
  - プラットフォーム間の差異（macOS/Linux/Windows）を吸収するため、必ず `TestDriver` インターフェースを使用してください。
  - シナリオ内に `process.platform` 分岐や、Playwright/Electron の生API (`page`, `_electron`) を記述することを禁止します。
2. **API First原則**:
  - 不安定なGUI操作（メニュークリックやショートカットキーによる操作）よりも、VSCode API (`vscode.commands.executeCommand` 等) の実行を優先してください。
  - これにより、外部要因によるテストのFlakinessを排除します。
3. **Structured Test Procedure (`runTest`)の強制**:
  - 従来の `Action-Verification` パターンをコードレベルで強制するため、必ず `await driver.runTest(...)` を使用してください。
  - `action` コールバックで操作を行い、その結果を受け取って `verifyLogic` コールバックで検証を行います。
  - `driver.runCommand` などを直接トップレベルで呼び出して、「検証なし」で済ませることを禁止します。
4. **Verification Helperの使用**:
  - `shouldBeVisible`, `shouldHaveText` などの再利用可能な検証ロジックは `ide/extension/e2e/helpers/verifications.ts` からインポートして使用してください。
  - これらは `runTest` の `verifyLogic` ブロック内で使用することを想定しています。
5. **Log Verification**:
  - `runTest` の `verifyLog` は**必須**です。
  - **必須**: `ide/extension/e2e/helpers/log-verification.ts` から `expectLog`, `expectNoLog`, または `ignoreLog` をインポートして使用してください。
  - ログ検証をスキップする場合でも、明示的に `ignoreLog` を渡すことで、「ログを確認しない」という意思をコードに明示します。

## 10. プロパティパネル開閉 + FitView の事前条件ルール

Blue Print エディタ（`TaskEditor`）の E2E シナリオでは、操作対象の領域に応じて、
事前にプロパティパネルの開閉状態を整え、併せてキャンバスの `fitView()` を実行する
ことを必須とします。

### 10.1 ルール


| 操作対象                                                                                             | 事前条件                                  | 呼び出す API                                                  |
| ------------------------------------------------------------------------------------------------ | ------------------------------------- | --------------------------------------------------------- |
| プロパティパネル（右ペイン）内の要素                                                                               | パネルが **展開** されている + キャンバス fitView 済み  | `await driver.ensurePropertiesPanelOpenAndFitView()`      |
| タスクエディタ（ReactFlow キャンバス）で**座標ベース** のマウス操作（`driver.click(x,y)` / `mouseMove` / `dblClick(x,y)` 等） | パネルが **折り畳まれて** いる + キャンバス fitView 済み | `await driver.ensurePropertiesPanelCollapsedAndFitView()` |


> 注: セレクタベースの `locator.dragAndDrop(src, tgt, { force: true })` のように
> DOM 解決のみで動く操作は、キャンバス幅に依存しないため、本ルールの対象外。
> 不必要に `ensurePropertiesPanel*AndFitView` を呼び出すと、パネル開閉による
> レイアウト再計算と auto-save / React Flow 内部状態の race を引き起こすことがある。

### 10.2 理由

- プロパティパネル展開時、タスクエディタの横幅が圧迫され、ReactFlow 内部の
DPI / zoom 計算と Playwright のグローバル座標計算にズレが生じ、
クリック・ドラッグの判定が不安定になる。
- 逆にパネル折り畳み時は、右ペイン内の入力要素が `display: none` になり
操作不能となる。
- パネル幅の変化によりキャンバス上のノードがビューポート外へはみ出す場合がある。
`fitView()` をパネル整備後に **常に** 実行することで、全ノードを表示範囲に
収め、後続の座標ベース操作やノードクリックの安定性を高める。

### 10.3 実装位置

- `runTest` の `action` コールバック **先頭** で呼び出す。
- 1 つのテストで両領域を跨ぐ場合は、各操作の直前に該当 API を挿入してよい。
- パネル状態が既に目的と一致する場合、トグルクリックは **no-op**（副作用なし）。
ただし `fitView()` はその場合でも **常に実行** され、キャンバスのビューポートを
全ノードに合わせ直す。

### 10.4 例外

折り畳みトグル自身の挙動を検証するテスト（例: `UI: Properties Panel Collapse Toggle`,
`UI: Blue Print Resizable Layout`）では、**意図的に `ensurePropertiesPanel*AndFitView`
を呼ばない**。初期状態（パネル初期幅・初期ビューポート）を壊すと検証不能になるため。
コードにはその旨をコメントで明示すること。

### 10.5 実装例

```ts
// Canvas operation (drag/click nodes)
await driver.runTest(
    'Drag node on canvas',
    async () => {
        await driver.ensurePropertiesPanelCollapsedAndFitView();
        await driver.dragAndDrop('[data-id="node-1"]', '[data-id="zone-a"]');
    },
    // ...
);

// PropertiesPanel operation (edit property fields)
await driver.runTest(
    'Edit property alias',
    async () => {
        await driver.ensurePropertiesPanelOpenAndFitView();
        const root = await driver.getAppRoot();
        await root.locator('[data-testid="property-alias-input"]').fill('NewName');
    },
    // ...
);

// Mixed: canvas operation, then property editing
await driver.runTest(
    'Select node and edit',
    async () => {
        await driver.ensurePropertiesPanelCollapsedAndFitView();
        const root = await driver.getAppRoot();
        await root.locator('[data-id="node-1"]').click({ force: true });

        await driver.ensurePropertiesPanelOpenAndFitView();
        await root.locator('[data-testid="property-alias-input"]').fill('X');
    },
    // ...
);
```

## 11. テスト項目の設計と検証観点 (Test Item Design & Verification Perspective)

テストを実装する前に、**テスト項目の設計**を明確なタスクとして実施してください。
「全テストが成功した」ことが「システムが実際に正しく動作している」と同義であるために、以下のプロセスに従ってテスト項目を設計・検証します。

### 11.1 設計の目的

テスト項目設計の目的は、**「このテスト群が全て成功すれば、システムが実際に完全に動作していると言い切れる」**ことを論理的に示せる確認項目一覧を作ることです。

単に「関数を呼んで例外が出ない」「HTTP 200 が返る」だけでは不十分です。**システムがどう動作し、どんな応答をしていれば、本当に動いていると言い切れるか**を、複数の観点から検討してください。

### 11.2 ボトムアップの確認順序 (Bottom-Up Verification Order)

> [!IMPORTANT]
> **モックの使用を避け、末端の小さな機能から実際の動作を確認し、少しずつ大きな機能へと「実際に動作している」という確認を積み上げてください。**

「呼び出す側 → 呼び出される側」の関係が `A → B → C` のように成り立つ場合：

1. **まず C（末端）をテスト**する。C が「実際に動作している」と言い切れるレベルのテストを作成する。
2. **次に B をテスト**する。C が動作していることは Step 1 で確認済みなので、B のテストでは C の動作を前提として、B 自体の振る舞いが正しいことを検証する。
3. **最後に A をテスト**する。B, C が動作していることを前提に、A を通した全体の振る舞いを検証する。

この順序により、テスト失敗時に原因箇所が明確になり、問題の切り分けが容易になります。

```
依存関係:  A → B → C

テスト順序:
  Step 1: C のテスト → C が動作していることを確認
  Step 2: B のテスト → B + C が動作していることを確認
  Step 3: A のテスト → A + B + C（全体）が動作していることを確認
```

### 11.3 テスト項目の観点チェックリスト

テスト項目を設計する際、以下の観点を網羅しているかを確認してください：

| # | 観点 | 確認内容 |
|---|------|----------|
| 1 | **正常系の動作確認** | 主要な入力パターンに対して、期待する出力・副作用が得られるか |
| 2 | **異常系・境界値** | 不正入力、空入力、上限値に対して、適切なエラーが返るか |
| 3 | **外部連携の実動作** | 外部システム（DB, API, ファイルシステム等）との連携が実際に機能しているか |
| 4 | **データの一貫性** | 書き込んだデータが正しく読み出せるか。変換後のデータが元に戻せるか |
| 5 | **状態遷移の検証** | 操作前後で、システムの状態が期待通りに変化しているか |
| 6 | **設定・構成の反映** | コンフィグやアダプタの選択が、意図したとおりに適用されているか |
| 7 | **副作用の確認** | 処理の結果、意図しない副作用（ファイル残存、リソースリーク等）が発生していないか |

### 11.4 テスト項目のセルフレビュー (Mandatory Self-Review)

> [!CAUTION]
> テスト項目を確定する前に、**必ず以下のセルフレビューを実施してください。** セルフレビューなしでテスト実装に進むことは禁止します。

テスト項目一覧を作成したら、以下の問いに回答し、不足があれば項目を追加してください：

1. **網羅性の検証**: 「このテスト項目群が全て成功した場合、この機能が実際に動作していると言えるか？」
   - 言えない場合 → 何が不足しているかを特定し、項目を追加する。
2. **証拠の十分性**: 「各テスト項目は、動作していることの証拠を得られるレベルに充実しているか？」
   - 例えば「エラーが出ない」だけでなく「期待する値が返る」「期待する状態に変わる」を確認しているか。
3. **迂回・抜け道の排除**: 「テストが成功しても、実は別の経路で処理されている可能性はないか？」
   - 意図したアダプタ/ハンドラ/ルートが使用されていることを、テスト内で確認しているか。
4. **依存関係の整合性**: 「呼び出し先のテストが成功していなければ、呼び出し元のテストの成功に意味がない」という前提が崩れていないか。

セルフレビューの結果、十分であると判断した理由を、実装計画書のテスト項目セクションに簡潔に記載してください。

## 12. 全テスト完了後の総合判定プロセス (Post-Test Comprehensive Verdict)

全テストが成功した後、**それだけでは「システムが正しく動作している」と結論づけない**でください。
以下の総合判定プロセスを実施し、最終的な判断を行ってください。

### 12.1 目的

テストの実行結果を鵜呑みにせず、**テストでは想定しきれなかった問題**がないかを再検証します。
「全テスト成功」という事実と、「システムが実際に動作している」という結論の間のギャップを埋めるためのプロセスです。

### 12.2 チェック項目

全テスト完了後、以下の項目を一つずつ確認してください：

| # | チェック項目 | 確認内容 |
|---|------------|----------|
| 1 | **スキップされたテストの有無** | テストの過程で、条件分岐やエラーハンドリングにより事実上スキップされた処理がないか。ログに `SKIP`, `WARN`, `TODO` 等のマーカーが出ていないか。 |
| 2 | **部分的なエラーの見落とし** | テスト全体は成功しているが、テストログ内に `ERROR`, `WARN`, `panic`, `recovered` などの異常兆候がないか。成功判定に影響しない箇所でエラーが発生していないか。 |
| 3 | **迂回処理による偽成功** | フォールバック処理や retry が働いた結果として成功しているだけで、本来のパス（primary path）は失敗していないか。 |
| 4 | **アダプタ・コンフィグの誤適用** | テストで確認したかったアダプタ/プロバイダ/ハンドラが実際に使用されているか。コンフィグミスにより別のアダプタが適用されて「動いているように見えている」だけではないか。 |
| 5 | **テスト間の依存・順序問題** | テストの実行順序に依存して成功している項目がないか。単独実行しても同じ結果が得られるか。 |
| 6 | **カバレッジの妥当性** | 新規・変更した機能に対して、テスト項目が設計されているか。「既存テストが通ったから大丈夫」で新機能のテストが欠落していないか。 |
| 7 | **外部システムの状態** | テスト実行時の外部システム（DB, コンテナ, API）が正常な状態であったか。テスト前にセットアップが正しく完了していたか。 |

### 12.3 総合判定の実施

上記チェック項目の結果を踏まえ、以下のフォーマットで**正直に**総合判定を記述してください：

```markdown
### 総合判定結果

**判定**: ✅ 動作確認完了 / ⚠️ 条件付き確認完了 / ❌ 追加確認必要

#### テスト結果サマリ
- 全テスト数: [N] 件
- 成功: [N] 件
- 失敗: [N] 件
- 事実上スキップ: [N] 件

#### チェック項目の結果
| # | チェック項目 | 結果 | 備考 |
|---|------------|------|------|
| 1 | スキップされたテスト | ✅/⚠️/❌ | [詳細] |
| 2 | 部分的なエラー | ✅/⚠️/❌ | [詳細] |
| ... | ... | ... | ... |

#### 判定理由
[なぜこの判定としたかの論拠を記述。
 特に ⚠️ や ❌ の場合は、何が不足しており、
 どのような追加確認が必要かを明記する。]
```

> [!WARNING]
> **「全テスト成功 → 動作確認完了」と機械的に判定することは禁止します。**
> 各チェック項目を実際に確認し、問題がないことを根拠として示した上で、判定を下してください。
> 懸念事項がある場合は ⚠️（条件付き確認完了）として正直に記載し、追加で必要な確認事項を明示してください。

### 12.4 判定結果の記録

総合判定の結果は、実装計画書の検証セクション、またはウォークスルー（walkthrough）に記載してください。
この判定結果が、その実装が「実際に動作している」ことの最終的な論拠となります。

