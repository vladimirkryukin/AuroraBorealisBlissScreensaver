#!/usr/bin/env bash
set -euo pipefail

APP_NAME="AuroraBorealisBliss"
APP_BUNDLE="${APP_NAME}.app"
CONTENTS_DIR="${APP_BUNDLE}/Contents"
MACOS_DIR="${CONTENTS_DIR}/MacOS"
RESOURCES_DIR="${CONTENTS_DIR}/Resources"
LEGACY_BIN="myapp"
TMP_BIN="${APP_NAME}"

echo "Building macOS binary..."
go build -o "${TMP_BIN}" .

echo "Creating app bundle..."
rm -rf "${APP_BUNDLE}"
mkdir -p "${MACOS_DIR}" "${RESOURCES_DIR}"
mv "${TMP_BIN}" "${MACOS_DIR}/${APP_NAME}"

cat > "${CONTENTS_DIR}/Info.plist" <<'PLIST'
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>CFBundleDevelopmentRegion</key>
  <string>en</string>
  <key>CFBundleExecutable</key>
  <string>AuroraBorealisBliss</string>
  <key>CFBundleIdentifier</key>
  <string>com.auroraborealisbliss.screensaver</string>
  <key>CFBundleInfoDictionaryVersion</key>
  <string>6.0</string>
  <key>CFBundleName</key>
  <string>AuroraBorealisBliss</string>
  <key>CFBundlePackageType</key>
  <string>APPL</string>
  <key>CFBundleShortVersionString</key>
  <string>1.0.0</string>
  <key>CFBundleVersion</key>
  <string>1</string>
  <key>LSUIElement</key>
  <true/>
</dict>
</plist>
PLIST

echo "Done: ${APP_BUNDLE}"
echo "Run without Terminal:"
echo "  open \"${APP_BUNDLE}\""

# Cleanup legacy/default Go output if it exists.
rm -f "${LEGACY_BIN}"

