#!/usr/bin/env bash
# Build NWAFU Connect as an embeddable C library (cmd/libnwafuconnect).
#
#   scripts/build-lib.sh [host|macos|android|ios|all]
#
# Outputs go to build/. Each build also writes the matching C header
# (libnwafuconnect.h) next to the library.
#
# host     c-shared for the current OS/arch (.dylib / .so / .dll)
# macos    universal .dylib (arm64 + amd64, lipo)
# android  c-shared .so per ABI; needs ANDROID_NDK_HOME (NDK r26+), API 24+.
#          Copy build/android/<abi>/libnwafuconnect.so into
#          android/app/src/main/jniLibs/<abi>/ of the host app.
# ios      c-archive .a for device (arm64) and simulator (arm64), combined
#          into build/ios/NwafuConnect.xcframework. Needs Xcode.
#          Link it statically and load symbols via DynamicLibrary.process().
#          With only Xcode-beta installed, prefix DEVELOPER_DIR=/Applications/Xcode-beta.app/Contents/Developer.
#
# Windows (from a Windows host with MinGW or llvm-mingw):
#   go build -buildmode=c-shared -trimpath -ldflags "-s -w" \
#     -o build/windows/nwafuconnect.dll ./cmd/libnwafuconnect
# Linux:
#   go build -buildmode=c-shared -trimpath -ldflags "-s -w" \
#     -o build/linux/libnwafuconnect.so ./cmd/libnwafuconnect
set -euo pipefail

cd "$(dirname "$0")/.."
PKG=./cmd/libnwafuconnect
LDFLAGS="-s -w"
export CGO_ENABLED=1

build_host() {
  local ext
  case "$(go env GOOS)" in
    darwin) ext=dylib ;;
    windows) ext=dll ;;
    *) ext=so ;;
  esac
  mkdir -p build
  go build -buildmode=c-shared -trimpath -ldflags "$LDFLAGS" \
    -o "build/libnwafuconnect.$ext" "$PKG"
  if [ "$ext" = dylib ]; then
    install_name_tool -id @rpath/libnwafuconnect.dylib build/libnwafuconnect.dylib
  fi
  echo "built build/libnwafuconnect.$ext"
}

build_macos() {
  mkdir -p build/macos
  for arch in arm64 amd64; do
    GOOS=darwin GOARCH=$arch MACOSX_DEPLOYMENT_TARGET=11.0 \
      go build -buildmode=c-shared -trimpath -ldflags "$LDFLAGS" \
      -o "build/macos/libnwafuconnect-$arch.dylib" "$PKG"
  done
  lipo -create -output build/macos/libnwafuconnect.dylib \
    build/macos/libnwafuconnect-arm64.dylib build/macos/libnwafuconnect-amd64.dylib
  install_name_tool -id @rpath/libnwafuconnect.dylib build/macos/libnwafuconnect.dylib
  cp build/macos/libnwafuconnect-arm64.h build/macos/libnwafuconnect.h
  echo "built build/macos/libnwafuconnect.dylib (universal)"
}

build_android() {
  : "${ANDROID_NDK_HOME:?set ANDROID_NDK_HOME to an NDK install}"
  local api=24 host_tag
  case "$(uname -s)" in
    Darwin) host_tag=darwin-x86_64 ;;  # NDK ships x86_64 host tools; Rosetta runs them
    Linux) host_tag=linux-x86_64 ;;
    *) echo "unsupported host for NDK build" >&2; exit 1 ;;
  esac
  local tc="$ANDROID_NDK_HOME/toolchains/llvm/prebuilt/$host_tag/bin"
  # abi:GOARCH:GOARM:clang-target
  for spec in "arm64-v8a:arm64::aarch64-linux-android" \
              "armeabi-v7a:arm:7:armv7a-linux-androideabi" \
              "x86_64:amd64::x86_64-linux-android"; do
    IFS=: read -r abi goarch goarm target <<<"$spec"
    mkdir -p "build/android/$abi"
    GOOS=android GOARCH=$goarch GOARM=$goarm CC="$tc/${target}${api}-clang" \
      go build -buildmode=c-shared -trimpath -ldflags "$LDFLAGS" \
      -o "build/android/$abi/libnwafuconnect.so" "$PKG"
    echo "built build/android/$abi/libnwafuconnect.so"
  done
}

build_ios() {
  mkdir -p build/ios
  local sdk cc
  for variant in "iphoneos:arm64:ios" "iphonesimulator:arm64:ios"; do
    IFS=: read -r sdk goarch goos <<<"$variant"
    cc="$(xcrun --sdk "$sdk" --find clang) -isysroot $(xcrun --sdk "$sdk" --show-sdk-path) -arch $goarch"
    if [ "$sdk" = iphoneos ]; then cc="$cc -miphoneos-version-min=13.0"; else cc="$cc -mios-simulator-version-min=13.0"; fi
    mkdir -p "build/ios/$sdk"
    GOOS=$goos GOARCH=$goarch CC="$cc" \
      go build -buildmode=c-archive -trimpath -ldflags "$LDFLAGS" \
      -o "build/ios/$sdk/libnwafuconnect.a" "$PKG"
  done
  rm -rf build/ios/NwafuConnect.xcframework
  xcodebuild -create-xcframework \
    -library build/ios/iphoneos/libnwafuconnect.a -headers build/ios/iphoneos/libnwafuconnect.h \
    -library build/ios/iphonesimulator/libnwafuconnect.a -headers build/ios/iphonesimulator/libnwafuconnect.h \
    -output build/ios/NwafuConnect.xcframework
  echo "built build/ios/NwafuConnect.xcframework"
}

case "${1:-host}" in
  host) build_host ;;
  macos) build_macos ;;
  android) build_android ;;
  ios) build_ios ;;
  all) build_host; build_macos; build_android; build_ios ;;
  *) echo "usage: $0 [host|macos|android|ios|all]" >&2; exit 2 ;;
esac
