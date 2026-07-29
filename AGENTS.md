# construction

（説明は後で記入）

## 開発タスクの進め方
- タスク着手前に `~/Global-Obsidian/knowledge/INDEX.md` を読み、関連ノートを参照する。
- 開発タスクは `~/Global-Obsidian/knowledge/開発/` の関連ルールを必ず参照し、それに沿って進める。

## 参照方針
- 設計方法やセッション管理方法など**プロジェクト固有でない内容**は
  `~/Global-Obsidian/knowledge/開発/` を参照する。
- 設計書の編集やセッションのコンテキスト管理など**プロジェクト固有の開発タスク**では
  `~/Project-Obsidian/construction/` を参照する。

## ハンドオフ運用
- ハンドオフファイルは必ず `~/Project-Obsidian/construction/Handoffs/` 配下に作成する（`Sessions/` には置かない）。
- **セッション再開時は `~/Project-Obsidian/construction/Handoffs/` の最新（日付が最も新しい）ハンドオフファイルを参照**してから作業を始める。
- **セッションのコンテキスト使用量が 100% に近づいたら、タスクの完了を待たず、作業途中でも一度ハンドオフを作成してセッションを切り替える。切り替え後は最新のハンドオフを参照し、同じタスクを再開する。**

## 進捗共有の方針
- ツールを使うタスクでは、着手時と作業中の節目に commentary で短い進捗を共有する。
- bash コマンド全文や逐次ログは貼らず、ユーザーが判断・検証するために必要な要点だけを伝える。
- 最終回答は、それ単体で変更内容・検証結果・残課題が分かる形にする。

## 記憶の方針
- プロジェクトの記憶・知見・経緯は **Obsidian で一元管理**する（`~/Project-Obsidian/construction/` と `~/Global-Obsidian/knowledge/`）。
- Codex の Memory / Chronicle をプロジェクト知識の正本にしない。二重管理を避け、Obsidian を単一の真実の源とする。
- プロジェクトを跨いで再利用できる新しい知識を得た場合は、`~/Global-Obsidian/knowledge/` に記憶するかユーザーへ確認する。
