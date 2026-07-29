#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
repo_root="$(cd -- "${script_dir}/.." && pwd -P)"
env_file="${1:-${repo_root}/.env.production}"

if [[ $# -gt 1 ]]; then
  printf 'usage: %s [path-to-env]\n' "$0" >&2
  exit 2
fi
if [[ ! -f "${env_file}" ]]; then
  printf 'error: environment file not found: %s\n' "${env_file}" >&2
  exit 1
fi

domain=''
mysql_database=''
mysql_user=''
mysql_password=''
mysql_root_password=''
session_secret=''
seen='|'

while IFS= read -r line || [[ -n "${line}" ]]; do
  line="${line%$'\r'}"
  [[ -z "${line}" || "${line}" == \#* ]] && continue
  if [[ "${line}" != *=* ]]; then
    printf 'error: invalid line (expected KEY=VALUE): %s\n' "${line}" >&2
    exit 1
  fi
  key="${line%%=*}"
  value="${line#*=}"
  if [[ ! "${key}" =~ ^[A-Z][A-Z0-9_]*$ ]]; then
    printf 'error: invalid variable name: %s\n' "${key}" >&2
    exit 1
  fi
  if [[ "${seen}" == *"|${key}|"* ]]; then
    printf 'error: duplicate variable: %s\n' "${key}" >&2
    exit 1
  fi
  seen="${seen}${key}|"
  case "${key}" in
    DOMAIN) domain="${value}" ;;
    MYSQL_DATABASE) mysql_database="${value}" ;;
    MYSQL_USER) mysql_user="${value}" ;;
    MYSQL_PASSWORD) mysql_password="${value}" ;;
    MYSQL_ROOT_PASSWORD) mysql_root_password="${value}" ;;
    SESSION_SECRET) session_secret="${value}" ;;
    *)
      printf 'error: unsupported variable: %s\n' "${key}" >&2
      exit 1
      ;;
  esac
done < "${env_file}"

if [[ ! "${domain}" =~ ^([A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?\.)+[A-Za-z]{2,63}$ ]]; then
  printf 'error: DOMAIN must be a hostname without scheme, port, path, or trailing dot\n' >&2
  exit 1
fi
if [[ ! "${mysql_database}" =~ ^[A-Za-z0-9_]{1,64}$ ]]; then
  printf 'error: MYSQL_DATABASE must contain 1-64 letters, digits, or underscores\n' >&2
  exit 1
fi
if [[ ! "${mysql_user}" =~ ^[A-Za-z0-9_]{1,32}$ ]]; then
  printf 'error: MYSQL_USER must contain 1-32 letters, digits, or underscores\n' >&2
  exit 1
fi

validate_hex_secret() {
  local name="$1"
  local value="$2"
  if [[ ! "${value}" =~ ^[0-9A-Fa-f]{64,}$ ]]; then
    printf 'error: %s must contain at least 64 hexadecimal characters\n' "${name}" >&2
    exit 1
  fi
}

validate_hex_secret MYSQL_PASSWORD "${mysql_password}"
validate_hex_secret MYSQL_ROOT_PASSWORD "${mysql_root_password}"
validate_hex_secret SESSION_SECRET "${session_secret}"

if [[ "${mysql_password}" == "${mysql_root_password}" ]]; then
  printf 'error: MYSQL_PASSWORD and MYSQL_ROOT_PASSWORD must differ\n' >&2
  exit 1
fi

all_values="${domain}|${mysql_database}|${mysql_user}|${mysql_password}|${mysql_root_password}|${session_secret}"
shopt -s nocasematch
if [[ "${all_values}" =~ (replace|change[-_]?me|example|placeholder|construction-demo) ]]; then
  printf 'error: known placeholder or demo value detected\n' >&2
  exit 1
fi

printf 'production environment validation: ok\n'
