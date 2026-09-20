---
name: build-pipeline
description: Build, Test, and Verify Pipeline
disable-model-invocation: false
---


# Build and Verification Workflow

コードの変更後に安全性（テスト通過）と正当性（ビルド成功）を検証し、統合テストまで一貫して実行する。

> [!IMPORTANT]
> テスト実行の詳細ルール（Linux/Remote-SSH 対応、エラー修正フロー、タイムアウト方針等）は
> `{{policy:testing-rules}}` を参照すること。

## 1. Full Build & Unit Test

プロジェクト全体のビルドと単体テストを一括実行する。
統合テストは最新のビルド成果物に対して行う必要があるため、**必ずこのステップを先に通す**。

// turbo
./scripts/process/build.sh

> **Linux / Remote-SSH**: `./scripts/process/build.sh --skip-etc` を使うこと（詳細は `{{policy:testing-rules}}` Section 1 参照）。

## 2. Environment Setup

統合テストに必要なコンテナ環境をセットアップする（未起動の場合のみ）。

// turbo
./scripts/setup/setup_containers.sh

## 3. Integration & E2E Tests

全ての統合テストとE2Eテストを実行する。**Step 1 が成功している必要がある。**

// turbo
./scripts/process/integration_test.sh

> **Linux / Remote-SSH**: `xvfb-run -a` でラップすること（詳細は `{{policy:testing-rules}}` Section 1 参照）。

特定のカテゴリやテストのみを実行したい場合:

```bash
./scripts/process/integration_test.sh --categories xxx
./scripts/process/integration_test.sh --specify "TestNameRegex"
./scripts/process/integration_test.sh --resume
```

## 4. Fix Loop

テストが失敗した場合は、`{{policy:testing-rules}}` Section 3「エラー修正フロー」に従い修正する。

1. エラーログを確認し原因を特定
2. コードを修正
3. `--specify` で失敗テストのみ再実行
4. 通過後、`--resume` で残りのテストを再開

## 5. Final Check & Push

全てのテストが通過し、リグレッションがないことを確認したら `git push` する。
テストが失敗している状態ではプッシュしない。
