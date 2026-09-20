---
name: create-implementation-plan
description: Create Implementation Plan
disable-model-invocation: false
---


# 実装計画作成ワークフロー

このワークフローは、アイデア/仕様書 (`.../ideas/.../XXX-{Name}.md`) を元に、**以下のテンプレートを埋める手順で**、ルールに基づいた実装計画書 (`.../plans/.../YYY-{Name}.md`) を作成します。

## 1. 入力とルールの確認

1.  **入力ファイルの特定**:
    *   ユーザーが指定したファイル、または現在エディタで開いているファイルを「仕様書」として扱います。
    *   ファイル名の形式は通常 `[3桁の連番]-[名前].md` です。
2.  **ルールの読み込み**:
    *   `{{policy:planning-rules}}` を読み込みます。
3.  **ステータスの取得**:
    *   `scripts/utils/show_current_status.sh` を実行します。
    *   JSON出力から `phase`, `branch`, `next_plan_id` を取得します。
    *   以下、`[Phase]`, `[Branch]`, `[NextID]` とします。

## 2. 出力先の決定

1.  **出力ディレクトリの特定**:
    *   **Phase-Aware Path**: 基本的に `prompts/phases/[Phase]/branches/[Branch]/plans/` を出力先とします。
    *   このディレクトリが存在しない場合は作成します。
2.  **ファイル名の決定**:
    *   形式: `[NextID]-[名前].md`

## 3. 実装計画書の作成 (Filling the Template)

**以下のテンプレートをコピーし、各プレースホルダー `[...]` を具体的に埋めてください。**

> [!IMPORTANT]
> **Technical Inheritance Rule**:
> 仕様書(Source Specification)に含まれる具体的なロジック、計算式、定数、アルゴリズム、コードスニペット、**およびデータ構造定義（Struct/Interface）**は、決して**要約せず**、そのまま、あるいはさらに具体化してこの計画書に含めてください。
> *   仕様書内のコードブロックはすべて「実装すべき対象」とみなしてください。「参照用」として無視することは禁止です。
> *   「仕様書の通り実装する」という記述は**禁止**です。必ずロジックを再記述してください。

---
### Template Start
```markdown
# [ファイル名 (拡張子なし)]

> **Source Specification**: [仕様書の相対パス]

## Goal Description
[機能や変更の概要を簡潔に記述]

## User Review Required
[ユーザーの確認が必要な事項。なければ "None." と記述]

## Requirement Traceability

> **Traceability Check**:
> 仕様書(Specification)の要件・決定事項をリストアップし、この計画書のどこで対応するかをマッピングしてください。
> もし仕様書の要件をこの計画で実装しない（先送りする）場合は、その理由を明記してください。

| Requirement (from Spec) | Implementation Point (Section/File) |
| :--- | :--- |
| [Requirements text] | [e.g. "Proposed Changes > File A"] |

## Proposed Changes

[ファイル単位で変更点を記述。依存関係順（Interface -> Struct -> Logic）に並べること]

### [コンポーネント名 (e.g. ide/extension)]

#### [MODIFY/NEW] [ファイルパス](file://プロジェクトルートからの相対パス)
*   **Description**: [変更の概要]
*   **Technical Design**:
    *   [関数シグネチャやインターフェース定義の変更点]
    *   ```typescript
        // Pseudo-code or Function Signature
        function example(arg: Type): ReturnType { ... }
        ```
*   **Logic**:
    *   [仕様書から継承したロジックを具体的に記述]
    *   [例: "変数AにBを代入し、Cを計算する"]

## Step-by-Step Implementation Guide

[ファイル単位ではなく、**時間軸**に沿った具体的な作業手順リスト]
[**重要**: 検証計画の「前」に実施手順を確定させること]

1.  **[Step Name]**:
    *   Edit `[File Path]` to [Specific Action].
    *   [Specific Code-Level Instruction, e.g. "Add 'count' field to State struct"]
2.  **[Step Name]**:
    *   ...

[最後に、必ず Verification Plan の実行を指示すること]

## Verification Plan

### Automated Verification
[ここには自動テストコマンドのみを記述する]

1.  **Build & Unit Tests**:
    run the build script.
    ```bash
    ./scripts/process/build.sh
    ```

2.  **Integration Tests**:
    Run integration tests.
    ```bash
    ./scripts/process/integration_test.sh --specify "[Unique Test Case Name]"
    ```
    *   **Log Verification**: [ログで何を確認すべきか具体的に記述]

3.  **E2E Tests (新規/追加)**:
    新機能の動作を検証するE2Eテストコードを `tests/` 配下に追加する。
    手動コマンド実行による確認は、E2Eテストコード化の**代替にはならない**。
    既存の E2E テストインフラ (`tests/agentservice_e2e_test.go` のヘルパー関数等) を積極的に活用すること。
    E2E テストが不要な場合（純粋な内部リファクタリング等）は、その理由を明記すること。

    #### [NEW/MODIFY] [テストファイル名](file://tests/xxx_test.go)
    *   **テストケース**: [テスト関数名と検証内容]
    *   **検証ポイント**: [何が動作していれば成功か]

## Documentation

`prompts/specifications`フォルダ以下にある、既存の仕様書およびドキュメントの内容を解析し、本計画で影響を受けるものを最新の状態に更新します。

#### [MODIFY] [ファイル名](file://プロジェクトルートからの相対パス)
*   **更新内容**: [変更点]
```
### Template End
---

## 4. ファイルの保存

作成したファイルを指定されたディレクトリに保存します。
複数ファイルに分割する必要がある場合は、「継続計画について」セクションを末尾に追加して分割してください（従来のルールに従う）。

> [!IMPORTANT]
> **一括作成ルール**: 実装計画を複数の Part に分割する場合は、**全ての Part を一括で作成してから**ユーザーへレビュー依頼してください。1つの Part だけ作成して承認を待ち、その後に次の Part を作成する方式は禁止です。全 Part を先に作成することで、ユーザーが実装の全貌を把握してからレビューできます。

## 5. セルフレビューと完了確認

実装計画を完了とみなす前に、以下の観点でセルフレビューを行い、修正を行ってください。

1.  **要件対比チェック**: `Requirement Traceability` テーブルが全て埋まっており、仕様書の全要件（特に些細なロジック変更含む）が網羅されているか。
2.  **再現性チェック**: この計画書だけで、迷わず実装できる具体性があるか。
3.  **データ構造チェック**: 仕様書の構造体定義やデータモデルが、省略されずに計画に含まれているか。
4.  **テスト網羅性チェック (Platform Specific)**:
    *   (Go) 単体テストと統合テストが計画されているか。また単体か統合かについて、テスト内容による区分けは適切か。
    *   TDDで計画されているか。
5.  **統合テストの実行プランチェック**:
    *   `./scripts/process/integration_test.sh` は全てを実行すると非常に長い時間がかかりますので、関係のあるテストを選択的に実行すべきです。
        *   `--categories` 及び `--specify` を組み合わせたテスト実行コマンドを必ず明記してあるか。
    *   テスト範囲が適切かどうか、テストシナリオなどを分析して検証すること。
6.  **テスト項目設計のセルフレビュー**:
    *   `{{policy:testing-rules}}` の §11 に従い、テスト項目がボトムアップ順序で設計され、観点チェックリスト (§11.3) が網羅されているか。
    *   §11.4 のセルフレビュー（網羅性・証拠の十分性・迂回排除・依存関係）の結果が記載されているか。
7.  **総合判定プロセスの計画**:
    *   `{{policy:testing-rules}}` の §12 に従い、全テスト完了後に総合判定を実施する手順が検証計画 (Verification Plan) に含まれているか。
8.  **E2Eテストコード化チェック**:
    *   新機能の動作確認が「手動コマンド実行」だけで終わっていないか。
    *   `tests/` 配下にE2Eテストコードが計画されているか。
    *   既存のE2Eヘルパー (`startE2EServer`, `createE2ESession` 等) を活用できないか確認したか。
    *   E2Eテストが不要と判断した場合、その理由が Verification Plan に明記されているか。
