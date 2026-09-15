#!/usr/bin/env bash
# run:setup — 冪等 (idempotent) 的前置作業，不啟動任何服務。

set -euo pipefail

project_root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
config_root="${XDG_CONFIG_HOME:-${HOME}/.config}/agentsdk"

mkdir -p "$config_root/data" "$config_root/logs" "$project_root/tmp"
ln -sfn "$config_root" "$project_root/tmp/config"

printf 'agentsdk config link: %s -> %s\n' "$project_root/tmp/config" "$config_root"
