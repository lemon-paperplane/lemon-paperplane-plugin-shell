#!/usr/bin/env bash
# 构建插件可执行文件到 bin/
set -euo pipefail
cd "$(dirname "$0")"
go build -trimpath -ldflags "-s -w" -o bin/paperplane-plugin-shell ./cmd/paperplane-plugin-shell
