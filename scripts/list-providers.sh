#!/bin/bash
set -e

PROVIDER="${1:-}"

echo "=== Registered Providers ==="
go run . provider --list-providers
echo ""

if [ -n "$PROVIDER" ]; then
    echo "=== Models for Provider: $PROVIDER ==="
    go run . provider --list-models --provider "$PROVIDER"
else
    echo "=== Models for Each Provider ==="
    PROVIDERS=$(go run . provider --list-providers | tr ',' ' ')
    for p in $PROVIDERS; do
        echo "--- Provider: $p ---"
        go run . provider --list-models --provider "$p" 2>&1 || true
        echo ""
    done
fi
