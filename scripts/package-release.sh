#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
repo_root="$(cd -- "${script_dir}/.." && pwd -P)"
repo_name="$(basename -- "${repo_root}")"

if [[ $# -gt 1 ]]; then
  printf 'usage: %s [output.tar.gz]\n' "$0" >&2
  exit 2
fi
if [[ "${repo_name}" != "construction" ]]; then
  printf 'error: expected repository directory to be named construction, got %s\n' "${repo_name}" >&2
  exit 1
fi

required_files=(
  ".env.production.example"
  "docker-compose.prod.yml"
  "backend/Dockerfile"
  "backend/cmd/api/main.go"
  "backend/migrations/000001_initial_schema.up.sql"
  "backend/internal/delivery/http/assets/fonts/NotoSansJP-Regular.ttf"
  "frontend/Dockerfile.prod"
  "frontend/package-lock.json"
  "frontend/src/main.tsx"
  "frontend/nginx.prod.conf"
  "deploy/nginx/construction.conf.example"
  "docs/production-deployment.md"
  "scripts/validate-production-env.sh"
)
for required_file in "${required_files[@]}"; do
  if [[ ! -f "${repo_root}/${required_file}" ]]; then
    printf 'error: required release file is missing: %s\n' "${required_file}" >&2
    exit 1
  fi
done

# Only these paths are eligible for a release. Exclusions below are defense in
# depth for secret-like files accidentally created inside an allowed source tree.
release_paths=(
  "AGENTS.md"
  "README.md"
  ".gitignore"
  "docker-compose.yml"
  "docker-compose.prod.yml"
  "backend/.dockerignore"
  "backend/Dockerfile"
  "backend/go.mod"
  "backend/go.sum"
  "backend/cmd"
  "backend/internal"
  "backend/third_party"
  "frontend/.dockerignore"
  "frontend/Dockerfile"
  "frontend/Dockerfile.prod"
  "frontend/eslint.config.js"
  "frontend/index.html"
  "frontend/nginx.prod.conf"
  "frontend/package.json"
  "frontend/package-lock.json"
  "frontend/src"
  "frontend/tsconfig.app.json"
  "frontend/tsconfig.json"
  "frontend/tsconfig.node.json"
  "frontend/vite.config.ts"
  "deploy/nginx/construction.conf.example"
  "docs/production-deployment.md"
  "scripts/package-release.sh"
  "scripts/validate-production-env.sh"
)

temp_base="${TMPDIR:-/tmp}"
temp_base="${temp_base%/}"
temp_root="$(mktemp -d "${temp_base}/construction-release.XXXXXX")"
staging_root="${temp_root}/stage"
archive_path="${temp_root}/construction-release.tar.gz"
archive_list="${temp_root}/archive.list"
keep_temp_root=false

cleanup() {
  if [[ "${keep_temp_root}" != true && -d "${temp_root}" ]]; then
    case "$(basename -- "${temp_root}")" in
      construction-release.*) rm -rf -- "${temp_root}" ;;
      *) printf 'error: refusing to remove unexpected temporary path: %s\n' "${temp_root}" >&2 ;;
    esac
  fi
}
trap cleanup EXIT

mkdir -p "${staging_root}/construction"
tar -C "${repo_root}" -cf - \
  --exclude='.env*' \
  --exclude='*.pem' \
  --exclude='*.key' \
  --exclude='id_rsa' \
  --exclude='id_ed25519' \
  --exclude='*credential*' \
  --exclude='*credentials*' \
  --exclude='*secret*' \
  --exclude='*secrets*' \
  --exclude='*.sql' \
  --exclude='*.dump' \
  --exclude='*backup*' \
  --exclude='backups' \
  "${release_paths[@]}" | tar -C "${staging_root}/construction" -xf -

# Migration SQL is executable application source, so copy only the convention-
# constrained migration set after excluding arbitrary SQL everywhere else.
mkdir -p "${staging_root}/construction/backend/migrations"
cp "${repo_root}"/backend/migrations/[0-9]*.up.sql "${staging_root}/construction/backend/migrations/"
cp "${repo_root}"/backend/migrations/[0-9]*.down.sql "${staging_root}/construction/backend/migrations/"

# This is the only environment-shaped file intentionally shipped.
cp "${repo_root}/.env.production.example" "${staging_root}/construction/.env.production.example"

tar -C "${staging_root}" -czf "${archive_path}" construction
tar -tzf "${archive_path}" > "${archive_list}"

if grep -Eiv '^construction/\.env\.production\.example$|^construction/backend/migrations/[0-9]+_[A-Za-z0-9_]+\.(up|down)\.sql$' "${archive_list}" | \
  grep -Eiq '(^|/)\.env|(^|/)(id_rsa|id_ed25519)$|\.(pem|key|sql|dump)$|(^|/)[^/]*(credential|secret|backup)[^/]*(/|$)|(^|/)backups(/|$)'; then
  printf 'error: release archive contains a secret or backup candidate\n' >&2
  exit 1
fi

required_archive_entries=(
  "construction/.env.production.example"
  "construction/docker-compose.prod.yml"
  "construction/backend/cmd/api/main.go"
  "construction/backend/migrations/000001_initial_schema.up.sql"
  "construction/backend/internal/delivery/http/assets/fonts/NotoSansJP-Regular.ttf"
  "construction/frontend/src/main.tsx"
  "construction/frontend/package-lock.json"
  "construction/deploy/nginx/construction.conf.example"
  "construction/docs/production-deployment.md"
  "construction/scripts/package-release.sh"
  "construction/scripts/validate-production-env.sh"
)
for archive_entry in "${required_archive_entries[@]}"; do
  if ! grep -Fqx "${archive_entry}" "${archive_list}"; then
    printf 'error: required archive entry is missing: %s\n' "${archive_entry}" >&2
    exit 1
  fi
done

if [[ $# -eq 1 ]]; then
  output_arg="$1"
  if [[ "${output_arg}" = /* ]]; then
    output_path="${output_arg}"
  else
    output_path="$(pwd -P)/${output_arg}"
  fi
  output_dir="$(dirname -- "${output_path}")"
  if [[ ! -d "${output_dir}" ]]; then
    printf 'error: output directory does not exist: %s\n' "${output_dir}" >&2
    exit 1
  fi
  if [[ -e "${output_path}" ]]; then
    printf 'error: refusing to overwrite existing output: %s\n' "${output_path}" >&2
    exit 1
  fi
  mv -- "${archive_path}" "${output_path}"
  archive_path="${output_path}"
else
  keep_temp_root=true
fi

printf '%s\n' "${archive_path}"
