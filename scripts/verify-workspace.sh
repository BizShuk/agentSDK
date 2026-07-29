#!/usr/bin/env bash
#
# 跨 go.work 全部 module 執行 build + test。
# go test ./... 只涵蓋 root module；sample/* 各自是獨立 module。
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel)"
cd "${ROOT}"

# 直接從 go.work 讀取，避免腳本與 go.work 各存一份 module 清單。
MODULES="$(go list -m -f '{{.Dir}}' 2>/dev/null || true)"
if [[ -z "${MODULES}" ]]; then
	echo "verify-workspace: 無法從 go.work 取得 module 清單" >&2
	exit 1
fi

# go build ./... 在只解析到單一 main package 時會把執行檔寫進工作目錄，弄髒 repo。
# 一律導向暫存目錄。
BUILD_OUT="$(mktemp -d)"
trap 'rm -rf "${BUILD_OUT}"' EXIT

FAILED=0
while IFS= read -r dir; do
	[[ -z "${dir}" ]] && continue
	rel="${dir#"${ROOT}"/}"
	[[ "${dir}" == "${ROOT}" ]] && rel="."
	printf '=== %s\n' "${rel}"
	if ! (cd "${dir}" && go build -o "${BUILD_OUT}/" ./... && go test ./... -count=1 -timeout=120s); then
		echo "verify-workspace: ${rel} FAILED" >&2
		FAILED=1
	fi
done <<< "${MODULES}"

if [[ "${FAILED}" -ne 0 ]]; then
	echo "verify-workspace: 有 module 未通過" >&2
	exit 1
fi
echo "verify-workspace: 全部 module 通過"
