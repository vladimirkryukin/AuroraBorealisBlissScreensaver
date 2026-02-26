# macOS Build Guide

## Build `.app` (recommended)

From project root:

```bash
cd source
./build_macos_app.sh
```

This creates `AuroraBorealisBlissScreensaver.app` in `source/`.

Run without Terminal:

```bash
open "AuroraBorealisBlissScreensaver.app"
```

## Manual Build (raw binary)

```bash
cd source
go mod download
go build -v -o AuroraBorealisBlissScreensaver .
```

Note: launching the raw binary directly may appear as Terminal-launched process.
