# Aurora Borealis Bliss Screensaver (Windows `.scr`)

Open-source Windows screensaver based on a custom OpenGL fragment shader, written in Go.

![Logo](../assets/logo.png)

## Preview

![Screenshot 1](../assets/screenshot01.jpg)
![Screenshot 2](../assets/screenshot02.jpg)
![Screenshot 3](../assets/screenshot03.jpg)

## Quick Navigation

- [MIT License](LICENSE)
- [Windows Build Guide](BUILD_WINDOWS.md)
- [macOS Build Guide](BUILD_MACOS.md)
- [Windows app-like build script](../source/build_windows_app.bat)
- [macOS app build script](../source/build_macos_app.sh)
- [Main source file](../source/main.go)

## License

This project is distributed under the MIT License. See [LICENSE](LICENSE).

## Repository Layout

- [`assets/`](../assets/) - graphics only (`icon.png`, `icon.ico`, `logo.png`, screenshots)
- [`source/`](../source/) - Go source code and build/resource files
- [`docs/`](./) - documentation and license

## Build on Windows

From project root:

```bash
cd source
```

### 1) Prerequisites

- Go 1.25+ (or compatible)
- OpenGL 3.3 capable GPU and up-to-date video drivers

Optional tools for executable metadata/icon resources:

- `rsrc` (`go install github.com/akavel/rsrc@latest`)
- or `goversioninfo` (`go install github.com/josephspurrier/goversioninfo/cmd/goversioninfo@latest`)

### 2) Download dependencies

```bash
go mod download
```

### 3) Build screensaver

```bash
go build -v -ldflags "-H windowsgui" -o AuroraBorealisBlissScreensaver.scr .
```

### 4) Install as screensaver

- Copy `AuroraBorealisBlissScreensaver.scr` to `C:\Windows\System32\`
- Open Windows Screensaver settings and select it from the list

## Notes

- Configuration mode is supported via standard screensaver args:
  - `/s` - full screen
  - `/c` - config/about dialog
  - `/p <HWND>` - preview mode in Windows screensaver panel
- The shader in `shader.json` is intentionally obfuscated and comment-free.

## Run on macOS without Terminal

From project root:

```bash
cd source
./build_macos_app.sh
open "AuroraBorealisBlissScreensaver.app"
```

Use the generated `.app` bundle for launch.
Do not run `source/myapp` directly if you want no Terminal window.
