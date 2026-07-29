# construction

架空データで動く、現場工事の案件・帳票・日報・工程管理デモです。

## 起動

```bash
docker compose up --build -d
```

- フロントエンド: http://localhost:5173
- API health: http://localhost:8080/health
- PDF変換: Compose内部の Gotenberg 8.34.0 Chromium（ホストには公開しません）
- 管理者: `admin` / `construction-demo`
- 一般利用者: `misaki` / `construction-demo`
- 外注作業員: `takahashi` / `construction-demo`

初回起動時にMySQL migrationと架空の案件、見積、請求、日報、工程表を投入します。

見積書・請求書のPDFは、backendがDBから再計算した金額で自己完結HTMLを生成し、内部Gotenbergへ変換を依頼します。GotenbergにはHTMLとNoto Sans JPフォントだけをストリーミング送信し、download-from・webhook・外部IPへの通信は無効化しています。変換は backend / Gotenberg ともに同時2件に制限し、backend側が満杯の場合は `503` と `Retry-After: 1` を返します。停止時もPDF取得は `503` になります。

## 検証

```bash
# Set a writable build cache when the default Go cache is not writable.
export GOCACHE=/private/tmp/construction-go-cache

(cd backend && go test ./...)
(cd frontend && npm test && npm run lint && npm run build)
(cd backend && TEST_DATABASE_DSN='construction:construction@tcp(127.0.0.1:3306)/construction?charset=utf8mb4&parseTime=True&loc=UTC' \
  go test ./internal/delivery/http -run 'Test.*HTTPPersistenceRegression' -count=1)
(cd backend && MIGRATION_TEST_ROOT_DSN='root:root@tcp(127.0.0.1:3306)/mysql?charset=utf8mb4&parseTime=True&loc=UTC' \
  go test ./internal/delivery/http -run TestDemoSeedMigrationLifecycle -count=1)
```

最後のテストは一時スキーマを作成・削除し、fresh install、既存v7相当からv8への更新、down→up、全rollbackを実SQLで検証します。

## 本番公開

SFTP で x86_64 Ubuntu VPS へ転送し、Docker Compose をホスト Nginx / Certbot 配下で公開する手順は [本番デプロイ手順](docs/production-deployment.md) を参照してください。
