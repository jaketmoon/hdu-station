#!/bin/sh
set -eu
mkdir -p build/bin/licenses
cp docs/third-party/ZanaoMCP-LICENSE build/bin/licenses/
cp docs/third-party/PressStart2P-OFL.txt docs/third-party/ShareTechMono-OFL.txt docs/third-party/ArkPixel-OFL.txt build/bin/licenses/
if [ "$(uname -s)" != "Darwin" ]; then
  echo "Executable: build/bin/hdu-station"
  exit 0
fi
bundle="build/bin/HDU Station.app"
mkdir -p "$bundle/Contents/MacOS" "$bundle/Contents/Resources"
mkdir -p "$bundle/Contents/Resources/licenses"
cp build/bin/licenses/ZanaoMCP-LICENSE "$bundle/Contents/Resources/licenses/"
cp build/bin/licenses/PressStart2P-OFL.txt build/bin/licenses/ShareTechMono-OFL.txt build/bin/licenses/ArkPixel-OFL.txt "$bundle/Contents/Resources/licenses/"
cp build/bin/hdu-station "$bundle/Contents/MacOS/hdu-station"
cat > "$bundle/Contents/Info.plist" <<'PLIST'
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>CFBundleName</key><string>HDU Station</string>
<key>CFBundleDisplayName</key><string>HDU Station · 选课助手</string>
<key>CFBundleExecutable</key><string>hdu-station</string>
<key>CFBundleIdentifier</key><string>com.hduhelp.station.course</string>
<key>CFBundlePackageType</key><string>APPL</string>
<key>CFBundleVersion</key><string>0.2.0</string>
<key>CFBundleShortVersionString</key><string>0.2.0</string>
<key>NSHighResolutionCapable</key><true/>
<key>LSMinimumSystemVersion</key><string>13.0</string>
</dict></plist>
PLIST
codesign --force --sign - "$bundle"
echo "App: $bundle"
