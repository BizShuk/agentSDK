#!/bin/bash
set -e

# 取得 workspace 下 tmp 目錄的絕對路徑
WORKSPACE_TMP="$(pwd)/tmp"
mkdir -p "$WORKSPACE_TMP"

CONFIG_DIR="/Users/shuk/.config/agentsdk"

# 建立或更新符號連結 (symbolic link)
if [ -L "$CONFIG_DIR" ]; then
    echo "符號連結已存在：$CONFIG_DIR -> $(readlink "$CONFIG_DIR")"
elif [ -d "$CONFIG_DIR" ]; then
    echo "目錄已存在，將內容移至 $WORKSPACE_TMP..."
    cp -A "$CONFIG_DIR"/. "$WORKSPACE_TMP/" 2>/dev/null || cp -r "$CONFIG_DIR"/* "$WORKSPACE_TMP/" 2>/dev/null || true
    rm -rf "$CONFIG_DIR"
    ln -s "$WORKSPACE_TMP" "$CONFIG_DIR"
    echo "已建立符號連結：$CONFIG_DIR -> $WORKSPACE_TMP"
else
    mkdir -p "$(dirname "$CONFIG_DIR")"
    ln -s "$WORKSPACE_TMP" "$CONFIG_DIR"
    echo "已建立符號連結：$CONFIG_DIR -> $WORKSPACE_TMP"
fi
