# 帳票用フォント

`NotoSansJP-Regular.ttf` は Google Fonts の Noto Sans Japanese から取得した Regular（可変ウェイト）1書体です。

- 出所: [Google Fonts / Noto Sans Japanese](https://github.com/google/fonts/tree/7ff85c87f93ea6cca5f41c69f2e4edcb90240f26/ofl/notosansjp)
- 固定取得元: `google/fonts` commit `7ff85c87f93ea6cca5f41c69f2e4edcb90240f26` の `NotoSansJP[wght].ttf`
- SHA-256: `c2f3b4d463500a2ddcd3849cded1fceeb9fd6d1c32e6cbecd568453ba50fc68f`
- ライセンス: SIL Open Font License 1.1（全文は [`OFL.txt`](OFL.txt)）
- 用途: 見積書・請求書 PDF に UTF-8 の日本語を埋め込むためだけに利用します。

実行時にシステムフォントを参照しないよう、Go バイナリへ `go:embed` で同梱します。フォントの再配布・PDFへの埋め込みは OFL-1.1 の条件に従います。
