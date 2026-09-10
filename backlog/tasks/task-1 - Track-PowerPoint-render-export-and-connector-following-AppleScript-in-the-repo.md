---
id: TASK-1
title: Track PowerPoint render-export and connector-following AppleScript in the repo
status: Done
assignee: []
created_date: '2026-09-08 21:28'
updated_date: '2026-09-10 09:12'
labels: []
milestone: m-3
dependencies: []
ordinal: 1000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Verifying generated decks needs PowerPoint's own rendering engine. Structural XML checks miss two classes of defect that only appear once PowerPoint lays the slide out: labels it re-wraps, and shapes whose coordinates collapse. A working batch pptx-to-PDF exporter already exists, but it lives outside version control as a compiled .scpt, so it cannot be reviewed, diffed, or re-used by anyone else.

Two capabilities are wanted:

1. Batch render export. Walk a folder of .pptx, export each one to PDF through PowerPoint, skip PowerPoint's ~$ temp files and already-exported PDFs, and report converted/skipped/failed counts.

2. Connector-following test. Open a .pptx, move one node shape by a fixed offset, save, re-export to PDF, and report whether the connector endpoints moved with the shape. This is the only way to confirm that stCxn/endCxn actually binds in PowerPoint: a static PDF or PNG shows the shapes but never shows whether the line follows. The claim that connectors stay attached is currently unverified against the application itself.

Store the script as plain-text .applescript rather than a compiled .scpt, so that changes are reviewable in a diff. osascript runs plain text directly, and osacompile can produce a .scpt when a double-clickable droplet is wanted. An existing compiled script can be recovered with osadecompile.

The repo has no scripts directory yet; this would create one.

This must stay an optional manual step. It requires macOS plus Microsoft PowerPoint, so it cannot become a dependency of go test, which has to keep running on any platform with only the Go toolchain.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 A plain-text AppleScript under scripts/ exports every .pptx under a given folder to a PDF beside the source, skipping ~$ temp files and existing PDFs, and reports converted/skipped/failed counts
- [x] #2 A connector-following mode moves a chosen shape in a .pptx by a given offset, saves a copy, re-exports the PDF, and reports each connector endpoint's position before and after the move
- [x] #3 Both run via osascript from the repo root with no compilation step
- [x] #4 Neither script is reachable from go test; go test still passes with no macOS or PowerPoint present
- [x] #5 AGENTS.md documents the scripts as an optional manual verification step, alongside the existing python-pptx inspection notes
- [x] #6 The connector-following result for a flowchart sample is recorded: whether the line endpoints move with the node in PowerPoint
<!-- AC:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. scripts/ を新設し、プレーンテキスト .applescript を 2 本置く（AC #4 の「Neither script」に合わせ、
   バッチ出力とコネクタ追従を別ファイルに分ける）。
2. scripts/export-pdf.applescript — 引数のフォルダ内 *.pptx を走査し、PowerPoint 経由で
   隣に PDF を出力。~$ 前置きの一時ファイルと既存 PDF はスキップ。converted/skipped/failed を集計。
   ファイル走査は do shell script (find) で行い、Finder / System Events への追加の自動化許可を要求しない。
3. scripts/connector-following.applescript — 引数 <file.pptx> <shape name> <dx> <dy>。
   移動前に全コネクタの端点を再構成 → 指定シェイプを移動 → 別名 pptx で保存 → PDF 出力 →
   移動後の端点を再構成し、コネクタごとに before/after と FOLLOWED/STATIC を報告。
4. 端点の再構成は convert_test.go の reconstructConnector と同じ式
   (flipH/flipV を戻し、box 中心まわりに rot を適用) を AppleScript 側で pt 単位で再現する。
   Go 側の期待値と同じ規約で読むことに意味があるため、独自の近似を作らない。
5. AGENTS.md / AGENTS.ja.md の「Verifying generated output」に、python-pptx の記述と並べて
   任意の手動検証手順として追記。go test からは到達しない（scripts/ に .go を置かない）ことを明記。
6. graph1 で実際に流し、AC #6 の追従結果（端点がノードに追従したか）を Implementation Notes に記録。

検証: gofmt -l . / go vet ./... / go test ./... （Go 側は無変更だが AC #4 の裏取りとして実行）、
および osascript での実行結果。
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
scripts/ を新設し、プレーンテキスト AppleScript を 2 本追加した。

- scripts/export-pdf.applescript (AC #1) — 引数フォルダ直下の *.pptx を PowerPoint 経由で
  隣に PDF 出力。`~$` ロックファイルと既存 PDF はスキップし converted/skipped/failed を集計。
- scripts/connector-following.applescript (AC #2) — 指定ノードを pt 単位で移動し、
  <deck>-moved.pptx と同 PDF を書き出して全コネクタ端点の移動前後を報告する。

1 本のスクリプトにモードを持たせるのではなく 2 本に分けた。AC #2 の文言は「mode」だが
AC #4 が「Neither script」と複数形で、責務も引数の形も別 (フォルダ / ファイル+シェイプ名+
オフセット) なので、ディスパッチを足すより分けたほうが読みやすい。

端点の再構成は convert_test.go の reconstructConnector と同じ式 (flipH/flipV を戻し
box 中心まわりに rot を適用) を pt 単位で再現した。Go 側と別の近似を作ると、
食い違ったときにどちらが誤りか切り分けられなくなるため。

### AC #6 の結果 — コネクタは PowerPoint 上でノードに追従する

sample/graph1.pptx の VALID (菱形) を (40, -30) pt 動かした実測:

    edge INPUT-VALID  end   (302.06, 242.45) -> (342.5, 212.45)   shifted (40.45, -30.0)
    edge VALID-PROC   begin (425.0, 231.32)  -> (476.57, 212.45)  shifted (51.58, -18.87)
    edge VALID-ERR    begin (415.14, 263.44) -> (476.57, 212.45)  shifted (61.44, -50.99)
    endpoints bound to VALID: followed=3 stuck=0
    other endpoints: unchanged=12 moved=1

VALID に接続された 3 端点すべてが移動した。つまり stCxn/endCxn は PowerPoint 側で
実際にバインドされている (従来これはアプリケーションに対して未検証だった)。

移動量が (40, -30) と一致しないのは、接続済み端点がその時点で正対する接続サイトへ
再ルーティングされるため。したがって判定は「オフセットぴったり動いたか」ではなく
「動いたかどうか」で行っている。`other endpoints: moved=1` は edge VALID-ERR の ERR 側で、
コネクタ全体が再ルーティングされた結果であって異常ではない。

### 検証

gofmt -l . (出力なし) / go vet ./... / go test ./... すべて green。
windows-amd64 と linux-amd64 のクロスコンパイルも通る。AC #4 の裏取りとして
`grep -rn "osascript|applescript|scripts/" --include=*.go .` が 0 件であることを確認した
(Go 側は今回一切変更していない)。
AC #1/#3 は sample/ 全 8 deck で実行し converted=8 failed=0、2 回目は skipped=8 を確認。

### 詰まった AppleScript 側の癖 (スクリプト内にもコメントとして残した)

- `save p in "path" as save as PDF` は成功を返して何も書かない。`POSIX file` 必須。
- `do shell script` の出力行区切りは linefeed ではなく carriage return。
- `result` / `before` / `by` / `my` / `line` は予約語。`tell` ブロック内では `rows` と `end` が
  PowerPoint のテーブル語彙に解決されるため、リスト構築は `tell` の外で行う。
- 素の AppleScript に三角関数がない (`sin of` は scripting addition が要る) ため
  sinOfDegrees を自前実装した。現状ジェネレータが出す rot は 0 と 90 度だけだが、
  角度が増えたときに黙って壊れないよう一般の角度で正しくしてある。

### AC 外で変えた点

.gitignore に /sample/*.pdf, /sample/*-moved.pptx, ~$*.pptx を追加した。
AGENTS.md に載せた手順をそのまま実行すると sample/ に未追跡ファイルが 9 個できるため。
<!-- SECTION:NOTES:END -->
