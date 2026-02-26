# Windows Build Guide

## Windows 11 ARM toolchain setup

If you build on a Windows 11 ARM machine and use CGO-based dependencies (OpenGL/GLFW), install a native ARM64 C toolchain first.

Recommended option: **MSYS2 CLANGARM64**.

### 1) Install MSYS2

- Download and install MSYS2 from: `https://www.msys2.org/`
- Default path is typically: `C:\msys64`

### 2) Install ARM64 compiler packages

Open **MSYS2 CLANGARM64** terminal and run:

```bash
pacman -Syu
pacman -S --needed base-devel mingw-w64-clang-aarch64-toolchain mingw-w64-clang-aarch64-binutils
```

### 3) Add compiler to PATH (PowerShell)

```powershell
$env:Path = "C:\msys64\clangarm64\bin;$env:Path"
clang --version
```

If `clang --version` works, toolchain is ready.

### 4) Build for Windows ARM64

From project root:

```powershell
cd source
$env:CGO_ENABLED="1"
$env:GOOS="windows"
$env:GOARCH="arm64"
$env:CC="clang"
go mod download
go build -v -ldflags "-H windowsgui" -o AuroraBorealisBlissScreensaver.scr .
```

You can remove temporary env vars in current shell:

```powershell
Remove-Item Env:GOOS, Env:GOARCH, Env:CC, Env:CGO_ENABLED
```

## Build `.scr`

```bash
cd source
go mod download
go build -v -ldflags "-H windowsgui" -o AuroraBorealisBlissScreensaver.scr .
```

## Build app-like package

```bat
cd source
build_windows_app.bat
```

This creates `source/AuroraBorealisBlissScreensaver-windows-app/` with:

- `AuroraBorealisBlissScreensaver.scr`
- `AuroraBorealisBlissScreensaver.cmd` (launcher in `/s` mode)

## Optional: add icon/version metadata

### Option A: `rsrc`

```bash
rsrc -ico ../assets/icon.ico -arch arm64 -o rsrc.syso
go build -v -ldflags "-H windowsgui" -o AuroraBorealisBlissScreensaver.scr .
```

### Option B: `goversioninfo`

```bash
goversioninfo -64 -o rsrc.syso versioninfo.json
go build -v -ldflags "-H windowsgui" -o AuroraBorealisBlissScreensaver.scr .
```

## Usage arguments (Windows screensaver protocol)

- `/s` - run fullscreen screensaver
- `/c` - open config/about dialog
- `/p <HWND>` - run embedded preview in screensaver settings panel
