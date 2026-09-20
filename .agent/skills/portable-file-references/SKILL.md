---
name: portable-file-references
description: Markdown ドキュメントを作成・編集する際に、ファイルへの参照を ワークスペースルートからの相対パスで記述し、特定の開発者環境に 依存しないポータブルな形式にする規約。
paths:
    - '**/*.md'
disable-model-invocation: false
---


# Markdown 内ファイル参照のポータビリティ規約

このプロジェクトの Markdown ドキュメントは、Git リポジトリを通じて複数の開発者・複数の環境
(Windows, macOS, Linux, CI)で共有される。ドキュメント内のファイル参照が特定の環境に
依存していると、他の開発者がドキュメントを読む際にパスが無効になり、情報の追跡が困難になる。

この capability は、Markdown ドキュメント内でリポジトリ内のファイルを参照する際に、
ポータブルな形式を使用するための規約を定義する。

## 規約

### 禁止: 絶対パスおよび file:// スキームの使用

以下の形式はリポジトリに記録される Markdown ドキュメントでは使用してはならない。

```markdown
<!-- 禁止: 絶対パス + file:// スキーム -->
[仕様書](file:///c:/Users/yamya/myprog/vv5/work/feat-minimum-vv/prompts/phases/refs/drafts/014-agent_procedure_integration_spec.md)

<!-- 禁止: Windows 絶対パス -->
[db.go](file:///c:/Users/yamya/myprog/vv5/work/feat-minimum-vv/features/chord/internal/store/db.go)

<!-- 禁止: バックスラッシュのパス -->
`c:\Users\yamya\myprog\vv5\work\feat-minimum-vv\features\chord\internal\store\db.go`
```

これらの形式は、ドキュメント作成者のローカル環境でしか通用しない。
チェックアウト先ディレクトリが異なる開発者や、OS が異なる環境ではパスが無効になる。

### 推奨: ワークスペース相対パスをインラインコードで記述

リポジトリ内のファイルを参照する場合は、ワークスペースルート(リポジトリルート)からの
相対パスを、バッククォートで囲んだインラインコードとして記述する。

```markdown
詳細は `prompts/phases/refs/drafts/014-agent_procedure_integration_spec.md` を参照。

Go 側の実装は `features/chord/internal/store/db.go` を参照。
```

### 表や箇条書き内での記述

表の中や箇条書きでも、同じ形式を使用する。

```markdown
| 先行仕様書 | 関係 |
| :--- | :--- |
| `prompts/phases/000-foundation/branches/feat-minimum-vv/ideas/032-subgraph-actor-generalization.md` | 子 Actor モデルは本仕様で継続利用 |

- 先行仕様: `prompts/phases/000-foundation/branches/feat-minimum-vv/ideas/032-subgraph-actor-generalization.md`
- スキーマ定義: `prompts/manifest/schemas/capability.schema.json`
```

### 行番号付きの参照

特定の行範囲を示したい場合は、パスの後に行番号情報をテキストで付記する。

```markdown
`features/chord/internal/runtime/quickjs.go` (L51-L154) の bridgeCode を参照。
```

### パス区切り文字

パス区切り文字にはスラッシュ (`/`) を使用する。
バックスラッシュ (`\`) は使用しないこと。
スラッシュであれば Windows / macOS / Linux のいずれでも正しく解釈できる。

### Markdown リンク記法の使い分け

| 対象 | 形式 | 例 |
| :--- | :--- | :--- |
| リポジトリ内のファイル | バッククォートの相対パス | `` `prompts/phases/refs/drafts/014-spec.md` `` |
| 外部 URL (HTTP/HTTPS) | Markdown リンク記法 | `[Go公式ドキュメント](https://go.dev/doc/)` |

Markdown リンク記法 `[text](url)` は、リポジトリ外の URL に対してのみ使用する。

## 適用範囲

この規約は、Git リポジトリに記録される全てのマークダウンファイルに適用する。

以下はこの規約の適用外とする:

- エージェントがチャット上で応答する際の一時的なリンク(IDE が解釈する `file://` リンク)
- CI/CD スクリプト内のパス指定
- コード内のファイルパス定数

