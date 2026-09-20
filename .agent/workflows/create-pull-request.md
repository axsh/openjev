---
name: create-pull-request
description: Create a pull request from committed code changes and manage post-merge revisions.
disable-model-invocation: false
---


# PR作成ワークフロー (Create Pull Request Workflow)

このワークフローは、実装したコードのコミットから、リモートリポジトリへのプッシュ、およびGitHub CLI (`gh` コマンド) を用いたPull Request (PR) の作成手順を定めます。また、PRマージ後に追加の改修が発生した際の運用ルールも定義します。

## 1. 変更の確認とコミット (漏れ確認と小口化)

コードのコミットは本来、実装の各ステップ（関数の追加ごと、テストの追加ごとなど）で細かく実施されていることが理想です。ここでのコミット作業は、**コミット漏れがないかの最終確認**と、残っている変更を整理してコミットするためのフォローアップとして位置付けられます。

1. **未コミットの変更を確認**:
   ```bash
   git status
   git diff
   ```
   まだコミットされていない変更がないか確認します。

2. **コミットの小口化 (Small Commits)**:
   未コミットのファイルが複数ある場合や、変更内容が多岐にわたる場合は、`git add .` で一度にすべてをコミットするのではなく、**意味のある論理的な小さな単位（機能追加、バグ修正、リファクタリングなど）に分割して** `git add <対象ファイル>` と `git commit` を繰り返すことを強く検討してください。

3. **コミットの実行**:
   コミットメッセージはプロジェクトの規則（英語）に従って記述します。
   **注意**: シェルからコマンドを実行する際、コミットメッセージ内のクォーテーション(`"`や`'`)、バッククォート(``` ` ```)、バックスラッシュ(`\`)、ドル記号(`$`) などのエスケープ処理に細心の注意を払ってください。途中で文字列が途切れないように適切にエスケープするか、複雑なメッセージの場合は一時ファイル(`tmp/commit_msg.txt`)に書き出してから `git commit -F tmp/commit_msg.txt` を使用することを推奨します。
   ```bash
   git commit -m "feat: add user authentication flow"
   ```

4. **コミットメッセージの確認**:
   コミット実行後、必ずログを確認してメッセージが文字化けや意図しない途切れ方をしていないか確認します。
   ```bash
   git log -1
   ```
   もし問題があれば `git commit --amend` で修正します。

5. **プッシュの実行**:
   現在のブランチをリモートにプッシュします。初回プッシュの場合は上流ブランチを設定します。
   ```bash
   git push -u origin <ブランチ名>
   ```

## 2. Pull Request の作成

プッシュが成功したら、GitHub CLI (`gh`) を使用してPRを作成します。

1. **環境変数のリセット**:
   `gh` コマンドが内部の認証システムと干渉しないよう、コマンド実行時に `GITHUB_TOKEN` を一時的に無効化して実行します。
2. **PR作成コマンドの実行**:
   タイトル(`--title`)と本文(`--body`)は**英語**で記述してください。本文には変更の概要、関連するイシュー、確認事項などを含めます。
   ```bash
   # bashを使用し、インラインで環境変数をクリアして実行する例
   GITHUB_TOKEN= gh pr create --title "Feature: User Authentication" --body "## Description
   Added the user authentication flow based on the specification.

   ## Changes
   - Implemented login component
   - Added token validation
   - Updated E2E tests"
   ```

3. **作成結果の確認**:
   コマンドが出力するPRのURLを確認し、作成が成功したことをユーザーに報告します。

## 3. 追加改修の運用フロー (PRマージ前/後)

状況に応じて、以下の運用ルールに従って改修をコミット・PRします。

### A. 対象のPRがまだオープン（マージ前）の場合
追加の改修やレビュー指摘の修正が必要になった場合は、**現在のブランチのまま**作業を続けます。
1. コードを修正
2. `git add` & `git commit`
3. `git push`
※ すでにPRが作成されているため、`git push` するだけで自動的にオープン中のPRに追加・反映されます。新たにPRを作成する必要はありません。

### B. 対象のPRがマージ済みの後に更なる改修が必要になった場合
マージ済みのブランチで作業を続けると、既存の変更を含んだ不要な差分が混じるため、**必ず新しいブランチを作成**して差分だけをPRします。
1. 最新の `main` (または `master`) を取得:
   ```bash
   git checkout main
   git pull origin main
   ```
2. 新しい作業ブランチを作成（例: `fix-auth-edge-cases`）:
   ```bash
   git checkout -b <新しいブランチ名>
   ```
3. コードを修正（追加改修の差分のみ実装）
4. 本ワークフローの「1. コミットとプッシュ」と「2. Pull Request の作成」を再実施し、新しいPRを作成します。
