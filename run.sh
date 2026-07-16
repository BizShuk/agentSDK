#!/bin/bash
set -e

# 取得 workspace 下 tmp 目錄的絕對路徑

mkdir -p "tmp"


ln -sf "${HOME}/.config/agentsdk" ./tmp/
