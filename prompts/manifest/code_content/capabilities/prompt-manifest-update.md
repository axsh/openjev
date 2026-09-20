---
apiVersion: agent.meta/v1
kind: capability
id: prompt-manifest-update
title: Prompt Manifest Update (Rules / Skills / Procedures)
description: >-
  ルール・スキル・ワークフロー等の Agent 向けプロンプトを改変する際の手順。
  prompts/manifest/code_content/ 以下のテンプレートを編集し、
  tt prompt update --target <target> で各コーディングエージェント
  （Cursor / Antigravity / Codex / Claude Code）の設定ディレクトリに反映する。
paths:
  - "prompts/manifest/**"
  - "prompts/manifest/code_content/**"
  - ".cursor/rules/**"
  - ".cursor/skills/**"
  - ".agent/rules/**"
  - ".agent/skills/**"
  - ".agent/workflows/**"
  - ".codex/rules/**"
  - ".codex/skills/**"
  - ".claude/rules/**"
  - ".claude/skills/**"
references:
  - "prompts/manifest/project.yaml"
  - "prompts/manifest/targets/cursor.yaml"
  - "prompts/manifest/targets/antigravity.yaml"
  - "prompts/manifest/targets/codex.yaml"
  - "prompts/manifest/targets/claude-code.yaml"
  - "prompts/manifest/schemas/capability.schema.json"
  - "prompts/manifest/schemas/policy.schema.json"
  - "prompts/manifest/schemas/procedure.schema.json"
body: inline
tags:
  - baseline

---

# Prompt Manifest Update（ルール・スキル・プロンプト改変手順）

Agent 向けの**ルール・スキル・ワークフロー（手順書）**を改変するときは、
各エージェントの設定ディレクトリ（`.cursor/`, `.agent/`, `.codex/`, `.claude/`）を直接編集せず、
**manifest テンプレートを編集してからデプロイ**する。

> [!CAUTION]
> 各エージェントの rules / skills は **`tt prompt update` により生成される**。
> テンプレートとの二重管理を避けるため、**正（canonical）は常に `prompts/manifest/code_content/`** とする。
> テンプレートを 1 回編集すれば、全エージェントに同一内容を配布できる。

## 1. 対応ターゲット

| ターゲット ID | エイリアス | エージェント | 出力先 |
|---------------|-----------|--------------|--------|
| `cursor` | — | Cursor | `.cursor/rules/`, `.cursor/skills/` |
| `antigravity` | `ag` | Antigravity | `.agent/rules/`, `.agent/skills/`, `.agent/workflows/` |
| `codex` | — | Codex | `.codex/rules/`, `.codex/skills/`（インデックス: `CODEX.md`） |
| `claude-code` | — | Claude Code | `.claude/rules/`, `.claude/skills/` |
| `all` | — | 上記すべて | 全ターゲットに一括デプロイ |

- ターゲット定義: `prompts/manifest/targets/{target}.yaml`
- `--target` 省略時は環境変数 `TT_TARGET`、未設定なら `all`
- **自分がどのエージェントとして動作しているかに応じて最低限そのターゲットへ反映**し、
  他エージェントと共用するリポジトリでは `all` での一括反映を推奨

## 2. ディレクトリ構成と種別

`prompts/manifest/code_content/` 以下の種別と、各ターゲットへの出力先の対応:

| 種別 | 配置 | スキーマ | 出力先（ターゲット共通の論理配置） | 役割 |
|------|------|----------|-----------------------------------|------|
| **policy** | `policies/*.md` | `schemas/policy.schema.json` | `{rules}/`（例: `.cursor/rules/*.mdc`, `.agent/rules/*.md`） | コーディング規範・テスト規範等（Agent ルール） |
| **capability** | `capabilities/*.md` | `schemas/capability.schema.json` | `{skills}/{id}/SKILL.md` | スキル（再利用可能な手順・知識） |
| **procedure** | `procedures/*.md` | `schemas/procedure.schema.json` | `{skills}/{id}/SKILL.md`（Antigravity は `{workflows}/`） | ワークフロー（`/build-pipeline` 等） |
| **refs** | `refs/*.md` | — | （参照のみ） | 補助ドキュメント |

ファイル拡張子・frontmatter 形式はターゲットごとにエミッタが変換する
（例: Cursor は `.mdc` + `alwaysApply`）。テンプレート側でターゲット固有の形式を書かない。

マニフェスト定義: `prompts/manifest/project.yaml`

```
prompts/manifest/
├── project.yaml              # ソースパス・出力設定
├── targets/                  # ターゲット別マッピング
│   ├── cursor.yaml
│   ├── antigravity.yaml
│   ├── codex.yaml
│   └── claude-code.yaml
├── schemas/                  # 各 kind の JSON Schema
└── code_content/
    ├── policies/             # → 各ターゲットの rules
    ├── capabilities/         # → 各ターゲットの skills
    ├── procedures/           # → 各ターゲットの skills / workflows
    └── refs/
```

## 3. 改変の種類別手順

### 3.1 ルール（policy）を改変する

1. 対象ファイルを特定: `prompts/manifest/code_content/policies/{id}.md`
2. YAML frontmatter が `policy.schema.json` に準拠していることを確認:
   - 必須: `apiVersion`, `kind`, `id`, `title`, `scope`, `activation`, `body`（または本文 inline）
   - 例: `id: coding-rules`, `scope: project`, `activation.mode: trigger`
3. 本文（`---` 以降）を編集
4. **§5 デプロイ** へ

### 3.2 スキル（capability）を新規追加・改変する

1. ファイル: `prompts/manifest/code_content/capabilities/{id}.md`
2. YAML frontmatter が `capability.schema.json` に準拠:
   - 必須: `apiVersion: agent.meta/v1`, `kind: capability`, `id`, `title`, `description`, `body`
   - `id` は kebab-case（例: `prompt-manifest-update`）
3. 本文に手順・制約・例を記述
4. 任意: `paths`, `references`, `scripts`, `manual_only`
5. **§5 デプロイ** へ

### 3.3 ワークフロー（procedure）を改変する

1. ファイル: `prompts/manifest/code_content/procedures/{id}.md`
2. YAML frontmatter が `procedure.schema.json` に準拠:
   - 必須: `apiVersion`, `kind`, `id`, `title`, `trigger`
   - `trigger.command`: スラッシュコマンド名（例: `build-pipeline`）
3. **§5 デプロイ** へ

## 4. 編集時の注意

- **直接編集禁止**: 各ターゲットの成果物（`.cursor/rules/*.mdc`, `.cursor/skills/*/SKILL.md`,
  `.agent/**`, `.codex/rules|skills/**`, `.claude/rules|skills/**`）
- **二重管理禁止**: テンプレートと成果物の内容が乖離しないよう、編集はテンプレートのみ
- **ターゲット非依存に書く**: テンプレート本文に特定エージェント固有の表現
  （「Cursor に反映する」「`.cursor/` に出力される」等）を書かない。
  出力先に言及する場合は「各エージェントの rules / skills」のように書く
- **スキーマ準拠**: 新規追加時は該当 `schemas/*.schema.json` の `required` を満たす
- **policy 間参照**: 本文中で `{{policy:coding-rules}}` 形式の参照が使える
- **言語**: `prompts/` 配下のテンプレート本文は **日本語**
- **レガシースクリプト**: `./scripts/code/prompt/compile.sh` 等は **`tt prompt`** に統合済み。通常は `tt prompt update` を使う

## 5. デプロイ（各エージェントへの反映）

### 5.1 実行前の確認（BEFORE）

デプロイ前に以下を記録し、変更検知のベースラインとする
（以下は Cursor の例。他ターゲットは §1 の出力先に読み替える）:

```bash
# メタデータ（前回デプロイ時刻・digest）
# Cursor: .cursor/.meta/last_update.yaml
# Antigravity: .agent/.meta/antigravity/last_update.yaml
cat .cursor/.meta/last_update.yaml

# 成果物のチェックサム
md5sum .cursor/rules/*.mdc .cursor/skills/*/SKILL.md 2>/dev/null | sort -k2

# ドライラン（変更予定の確認）
tt prompt update --target <target> --dry-run
```

### 5.2 デプロイ実行

```bash
# 単一ターゲット
tt prompt update --target cursor
tt prompt update --target ag           # antigravity のエイリアス
tt prompt update --target codex
tt prompt update --target claude-code

# 全ターゲット一括（推奨: 複数エージェント共用リポジトリ）
tt prompt update --target all
```

オプション:

| フラグ | 用途 |
|--------|------|
| `--dry-run` | ファイルを書き込まず計画のみ表示 |
| `--force` | 変更検知なしでも強制更新 |
| `--verbose` | 詳細ログ |
| `--report <path>` | 実行レポートを Markdown 出力 |

### 5.3 実行後の確認（AFTER）

以下を BEFORE と比較し、反映を検証する（Cursor の例）:

```bash
# メタデータが更新されたか
cat .cursor/.meta/last_update.yaml
# → updated_at, source_digest が変わっていること

# 対象ファイルのタイムスタンプ・チェックサム
md5sum .cursor/rules/*.mdc .cursor/skills/*/SKILL.md 2>/dev/null | sort -k2

# テンプレート本文との一致（policy / capability 例）
diff <(awk 'BEGIN{c=0} /^---$/{c++; next} c>=2' prompts/manifest/code_content/policies/coding-rules.md) \
     <(awk 'BEGIN{c=0} /^---$/{c++; next} c>=2' .cursor/rules/coding-rules.mdc) || true

# 新規スキルの存在確認（{id} は capability の id）
test -f .cursor/skills/{id}/SKILL.md && echo "OK: skill deployed"
```

**合格基準**:

1. `tt prompt update` が exit 0
2. 各ターゲットの `.meta/last_update.yaml` の `updated_at` / `source_digest` が更新
3. 編集したテンプレートに対応する成果物が存在し、本文が一致（frontmatter 形式差のみ許容）
4. 意図したキーワード・見出しが成果物に含まれる
5. `--target all` の場合、全ターゲットで 1–4 を満たす

## 6. トラブルシューティング

| 症状 | 対処 |
|------|------|
| `Update succeeded` だが内容が古い | `--force` で再実行。テンプレート保存忘れを確認 |
| スキーマエラー | frontmatter の必須フィールド・`id` の kebab-case を確認 |
| skills に新スキルが無い | `targets/{target}.yaml` で `capability: true` を確認 |
| policy が rules に無い | `targets/{target}.yaml` で `policy: true` を確認 |
| 特定ターゲットだけ反映されない | ターゲット ID の綴りを確認（`claude-code` はハイフン区切り。`claudecode` は不可） |
| 新ターゲットを追加したい | `prompts/manifest/targets/{id}.yaml` を作成し、`includes` / `paths` を定義 |

## 7. 作業完了時の報告

デプロイ後、ユーザーに以下を報告する:

1. 編集したテンプレートファイルのパス
2. 実行したターゲットと `tt prompt update` の成否
3. BEFORE / AFTER の `last_update.yaml` 差分
4. 反映された成果物パス（例: `.cursor/skills/prompt-manifest-update/SKILL.md`）
5. 本文一致確認の結果

## 8. 関連コマンド一覧

```bash
tt prompt --help
tt prompt update --help
tt prompt compile --help    # 中間成果物の確認用
tt prompt deploy --help     # compile + deploy
```

通常の開発フローでは **`tt prompt update --target <target>`**（または `--target all`）のみで十分。
