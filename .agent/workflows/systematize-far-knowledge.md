---
name: systematize-far-knowledge
description: Process pending intake events and compile far-knowledge into agent-readable skills.
disable-model-invocation: false
---


# 遠方知識の体系化ワークフロー

pending intake events を確認し、遠方知識をカテゴリ化・体系化・スキル化する。

## 実行タイミング

ユーザーが明示的に指示した時に実行する。

## 手順

### Step 1: Pending Events の確認

```bash
./scripts/code/agent/intake.sh list --status pending
```

pending events が 0 件の場合、以下を報告して終了する:

```text
Far-knowledge systematization: no pending events. Skipping.
```

### Step 2: 既存カテゴリツリーの確認

```bash
./scripts/code/agent/knowledge.sh list
```

### Step 3: 各 Pending Event の分析

各 pending event について:

1. `./scripts/code/agent/intake.sh show <event-id>` で内容を確認
2. 距離判定を実施:
   - この知識は、近傍コードを検索すれば得られるか? -> はいなら、スキップ
   - この知識は、無関係なモジュールの開発時にも適用できるか? -> カテゴリ化対象
3. 既存カテゴリとの関連を判定

### Step 4: カテゴリ判定と登録

各 event について:

**新規カテゴリが必要な場合:**

知識の内容をマークダウンファイルとして一時ファイルに書き出し、
`knowledge.sh add` でカテゴリを作成する:

```bash
# 一時ファイルに知識内容を書き出す
cat > tmp/knowledge_content.md << 'EOF'
# Knowledge Title

Knowledge content extracted from the intake event...
EOF

./scripts/code/agent/knowledge.sh add \
  --category-path "<category-name>" \
  --title "<knowledge-title>" \
  --description "<category-description>" \
  --content-file tmp/knowledge_content.md \
  --source-events "<event-id>"
```

**既存カテゴリに追記する場合:**

```bash
./scripts/code/agent/knowledge.sh append \
  --category-path "<existing-category>" \
  --title "<knowledge-title>" \
  --content-file tmp/knowledge_content.md \
  --source-events "<event-id>"
```

### Step 5: 再整理の判断

カテゴリが増えた場合、以下の基準で再整理を検討する:

1. **split が必要なとき**: 1つのカテゴリの知識ファイルが多すぎる(5件以上)、
   または内容が2つ以上の明確に異なるサブトピックを含む場合
2. **merge が必要なとき**: 2つのカテゴリの知識が頻繁に相互参照され、
   単独では不完全な場合
3. **rename が必要なとき**: カテゴリ名がその内容を正確に表現しなくなった場合
4. **move が必要なとき**: 特定の知識項目が現在のカテゴリよりも
   別のカテゴリに強く関連する場合
5. **何もしない**: 上記のいずれにも該当しない場合

再整理が必要な場合は `knowledge.sh` の split/merge/rename/move を使用する。

### Step 6: Processed への移行

処理済みの各 event を processed に移動する:

```bash
./scripts/code/agent/intake.sh processed <event-id>
```

### Step 7: スキル化の検討

カテゴリの内容をスキル化する。各カテゴリについて:

1. **カテゴリの知識ファイルを確認**:
   - 知識ファイルが 5 件以上ある場合、または複数の異なるサブトピックを含む場合は、Step 5 に戻りカテゴリを split（分割）するか、トピックごとに別々のスキルに分割する。
2. **知識の蒸留と統合（必須ルール）**:
   - **禁止: 知識ファイルの機械的な単純連結（cat）**:
     - `## 知識ファイル: <id>` のような内部ファイル名見出しをそのまま並べてはならない。
     - 各知識ファイルの個別 YAML フロントマター（`id`, `knowledge_id`, `category_path`, `source_event_ids`, `created_at`, `last_updated`, `status`）は**必ずすべて除去**すること。これらはナレッジストア管理用のメタデータであり、スキル本文に含めてはならない。
     - 本文中に `---`（二重フロントマターや余計な区切り線）を残してはならない。
   - **トピック別の再構成**:
     - カテゴリ内の知識群から本質的なパターン・判断基準・注意点を抽出し、自然な章立て（`# スキル名`, `## トピックA`, `## トピックB`）で再構成・要約する。
     - Coding Agent がタスク実行時に迷わず適用できるよう、具体的かつ簡潔な記述にする。
3. **capability スキーマへの変換**:
   - ファイル最上部に Capability フロントマターを **1つだけ** 定義する:
     - `apiVersion: agent.meta/v1`
     - `kind: capability`
     - `id`: `__far-knowledge-` プレフィックスを付与
     - `title`: 簡潔な英語/日本語タイトル
     - `description`: どのような場面で参照・適用すべきかの簡潔な要約
     - `user_visible: false`
     - `manual_only: false`
     - `status: current`
     - `body: inline`
4. **配置先**:
   - `prompts/memory/branches/<branch-package-id>/skills/<id>/SKILL.md` に配置
   - 重要: `<id>/SKILL.md` のサブディレクトリ構造にすること (フラットファイル不可)
   - `ScanBranchSkills()` がこの構造を期待する

フォーマット例:

```markdown
---
apiVersion: agent.meta/v1
kind: capability
id: __far-knowledge-sample-topic
title: "Far-Knowledge: Sample Topic"
description: >-
  Cross-cutting patterns and guidelines for Sample Topic.
user_visible: false
manual_only: false
status: current
body: inline
---

# Sample Topic

## Core Guidelines
- Guideline 1
- Guideline 2

## Specific Patterns
Explanation of the pattern...
```

配置例:

```
prompts/memory/branches/BR-xxx/skills/
  __far-knowledge-agent-record-branch-package/
    SKILL.md
  __far-knowledge-prompt-memory-architecture/
    SKILL.md
```

### Step 8: デプロイ

```bash
./scripts/code/prompt/update.sh
```

### Step 9: 結果報告

以下の形式で報告する:

```text
Far-knowledge systematization: completed.
- Processed events: <count>
- New categories: <list>
- Updated categories: <list>
- Reorganized: <details>
- Skills generated: <list>
- Deployed: yes/no
```

## 制約

- 本番コードの変更は行わない
- `prompts/memory/knowledge/` と `prompts/memory/branches/*/skills/` のみを変更対象とする
- 既存のユーザー管理スキル (`prompts/manifest/code_content/capabilities/`) は変更しない
