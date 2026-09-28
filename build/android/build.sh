#!/usr/bin/env bash
# 交叉编译安卓版引擎 CLI(Termux 可直接运行)
# 产物: build/android/zju-connect-android-arm64
set -euo pipefail

cd "$(dirname "$0")/../../zju-connect"

CGO_ENABLED=0 GOOS=android GOARCH=arm64 \
  go build -ldflags="-s -w" \
  -o ../build/android/zju-connect-android-arm64 .

echo "OK -> build/android/zju-connect-android-arm64"

# 构建 gomobile AAR(需 ANDROID_HOME + NDK;GOTOOLCHAIN 钉住已缓存版本避免代理查询)
gomobile bind -target=android -androidapi 24 -ldflags="-s -w" \
  -o ../build/android/zju-mobile.aar ./mobile
echo "OK -> build/android/zju-mobile.aar"
