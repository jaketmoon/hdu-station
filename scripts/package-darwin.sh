#!/bin/sh
set -eu

if [ "$(uname -s)" != "Darwin" ] || [ "$(uname -m)" != "arm64" ]; then
  echo "package-darwin requires macOS arm64" >&2
  exit 1
fi

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd -P)
project_dir=$(CDPATH= cd -- "$script_dir/.." && pwd -P)
stage_dir=$(mktemp -d "${TMPDIR:-/tmp}/hdu-station-package.XXXXXX")
verify_dir=$(mktemp -d "${TMPDIR:-/tmp}/hdu-station-verify.XXXXXX")

cleanup() {
  rm -rf "$stage_dir" "$verify_dir"
}
trap cleanup EXIT HUP INT TERM

mkdir -p "$stage_dir/project" "$project_dir/dist"

if ! git -C "$project_dir" diff --quiet HEAD -- || \
  ! git -C "$project_dir" diff --cached --quiet HEAD --; then
  echo "package-darwin packages committed HEAD; working tree changes are excluded" >&2
fi

git -C "$project_dir" archive --format=tar HEAD | tar -x -C "$stage_dir/project"

(
  cd "$stage_dir/project"
  export MACOSX_DEPLOYMENT_TARGET=13.0
  export CGO_CFLAGS="${CGO_CFLAGS:+$CGO_CFLAGS }-mmacosx-version-min=13.0"
  export CGO_CXXFLAGS="${CGO_CXXFLAGS:+$CGO_CXXFLAGS }-mmacosx-version-min=13.0"
  "${GO:-go}" run \
    "github.com/wailsapp/wails/v2/cmd/wails@${WAILS_VERSION:-v2.14.0}" \
    build -clean -platform darwin/arm64 -nosyncgomod
)

app_path="$stage_dir/project/build/bin/hdu-station.app"
archive_path="$project_dir/dist/hdu-station-darwin-arm64.zip"
test -d "$app_path"
plutil -lint "$app_path/Contents/Info.plist"
codesign --verify --deep --strict --verbose=2 "$app_path"
vtool -show-build "$app_path/Contents/MacOS/hdu-station" | grep -Eq 'minos[[:space:]]+13(\.0+)?'

rm -f "$archive_path"
ditto -c -k --keepParent --norsrc --noextattr --noqtn "$app_path" "$archive_path"
ditto -x -k "$archive_path" "$verify_dir"

verified_app="$verify_dir/hdu-station.app"
codesign --verify --deep --strict --verbose=2 "$verified_app"
plutil -lint "$verified_app/Contents/Info.plist"
if xattr -p com.apple.FinderInfo "$verified_app" >/dev/null 2>&1; then
  echo "packaged app contains FinderInfo" >&2
  exit 1
fi
if xattr -p com.apple.ResourceFork "$verified_app" >/dev/null 2>&1; then
  echo "packaged app contains a resource fork" >&2
  exit 1
fi
vtool -show-build "$verified_app/Contents/MacOS/hdu-station" | grep -Eq 'minos[[:space:]]+13(\.0+)?'

echo "$archive_path"
