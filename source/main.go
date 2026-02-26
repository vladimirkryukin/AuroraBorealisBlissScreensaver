// Aurora Borealis Bliss screensaver runtime.
//
// The executable supports all standard Windows screensaver modes:
//   - /s (or no args): fullscreen playback
//   - /c: settings/about dialog
//   - /p <HWND>: embedded preview in Windows screensaver control panel
//
// Rendering pipeline:
//  1. Load shader JSON from embedded `shader.json`.
//  2. Repair/minify shader code defensively (for malformed exports).
//  3. Build OpenGL program and draw a fullscreen quad each frame.
//  4. Populate common shader uniforms (`iTime`, `iResolution`, etc.).
package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"log"
	"math/rand"
	"os"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	"github.com/go-gl/gl/v3.3-core/gl"
	"github.com/go-gl/glfw/v3.3/glfw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

var iconPNGData []byte
var iconICOData []byte
var logoPNGData []byte

//go:embed shader.json
var shaderJSONData []byte

const (
	// Runtime behavior flags.
	// They are kept as compile-time constants so release builds stay predictable.
	FULLSCREEN_MODE           = true
	DEBUG_MODE                = false
	EXIT_ON_MOUSE_CLICK       = true
	EXIT_ON_KEY_PRESS         = true
	HIDE_MOUSE_CURSOR         = true
	FORCE_SETTINGS_MODE       = false

	// Product identity and UI strings.
	SCREENSAVER_NAME          = "Aurora Borealis Bliss Screensaver"
	CONFIG_WINDOW_TITLE       = "About"
	WEBSITE_URL               = "https://www.fullscreensavers.com/?utm_source=About&utm_medium=auroraborealisbliss"
	VISIT_WEBSITE_BUTTON_TEXT = "Visit website"
	COPYRIGHT_TEXT            = "© 2026 Aurora Borealis Bliss Screensaver contributors (MIT License)"
	WEBSITE_TEXT              = "More free screensavers on https://www.fullscreensavers.com"
	EMAIL_TEXT                = "Feel free to contact us: support@fullscreensavers.com"

	// Colors and styling constants
	ABOUT_TEXT_COLOR        = "#000000" // Black (for title)
	ABOUT_TEXT_FONT_SIZE    = 12        // Font size in points
	INFO_TEXT_COLOR         = "#0000FF" // Blue (for copyright, website, email)
	BUTTON_TEXT_COLOR       = "#FFFFFF" // White
	BUTTON_BACKGROUND_COLOR = "#0078D4" // Blue
	WINDOW_BACKGROUND_COLOR = "#90EE90" // Light green (like in the image)
)

// fixedSizeLayout - layout for fixed-size container
type fixedSizeLayout struct {
	width  float32
	height float32
}

func (l *fixedSizeLayout) Layout(objects []fyne.CanvasObject, containerSize fyne.Size) {
	if len(objects) > 0 {
		obj := objects[0]
		// Limit content size to container size
		obj.Resize(fyne.NewSize(l.width, l.height))
		obj.Move(fyne.NewPos(0, 0))
	}
}

func (l *fixedSizeLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(l.width, l.height)
}

// dialogLayout - layout for dialog with precise padding control
type dialogLayout struct {
	width         float32
	height        float32
	topPadding    float32
	bottomPadding float32
	spacing       float32
}

func (l *dialogLayout) Layout(objects []fyne.CanvasObject, containerSize fyne.Size) {
	if len(objects) < 3 {
		return
	}

	currentY := l.topPadding

	// Title label (index 0) - use fixed size for visibility
	titleLabel := objects[0]
	titleSize := fyne.NewSize(l.width-40, 25)
	titleLabel.Resize(titleSize)
	titleLabel.Move(fyne.NewPos((l.width-titleSize.Width)/2, currentY))
	currentY += titleSize.Height + l.spacing

	// Logo image (index 1) - use size from logoLayout, but limit if needed
	logoImage := objects[1]
	logoSize := logoImage.MinSize()
	// Maximum height for logo: remaining space minus text lines and button
	textLinesHeight := float32(0)
	if len(objects) >= 6 {
		textLinesHeight = 20 * 3 // 3 text lines (copyright, website, email) with spacing
	}
	maxAvailable := l.height - currentY - l.bottomPadding - l.spacing - 40 - textLinesHeight // 40px for button
	if logoSize.Height > maxAvailable {
		logoSize.Height = maxAvailable
	}
	if logoSize.Width > l.width/2 {
		logoSize.Width = l.width / 2
	}
	// If height is greater than width, make it square
	if logoSize.Height > logoSize.Width {
		logoSize.Height = logoSize.Width
	}
	logoImage.Resize(logoSize)
	logoImage.Move(fyne.NewPos((l.width-logoSize.Width)/2, currentY))
	currentY += logoSize.Height + l.spacing

	// Text lines (copyright, website, email) - indices 2, 3, 4
	textSpacing := float32(5) // Smaller spacing between text lines
	for i := 2; i <= 4 && i < len(objects); i++ {
		textLabel := objects[i]
		textSize := fyne.NewSize(l.width-40, 20)
		textLabel.Resize(textSize)
		textLabel.Move(fyne.NewPos((l.width-textSize.Width)/2, currentY))
		currentY += textSize.Height + textSpacing
	}

	// Button (last element) - use minimum size
	buttonIdx := len(objects) - 1
	if buttonIdx >= 0 {
		button := objects[buttonIdx]
		buttonSize := button.MinSize()
		if buttonSize.Width > l.width-40 {
			buttonSize.Width = l.width - 40
		}
		if buttonSize.Height < 30 {
			buttonSize.Height = 35
		}
		button.Resize(buttonSize)
		button.Move(fyne.NewPos((l.width-buttonSize.Width)/2, currentY))
	}
}

func (l *dialogLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(l.width, l.height)
}

// logoLayout - simple layout for limiting logo width
type logoLayout struct {
	width float32
}

func (l *logoLayout) Layout(objects []fyne.CanvasObject, containerSize fyne.Size) {
	if len(objects) > 0 {
		obj := objects[0]
		obj.Resize(fyne.NewSize(l.width, l.width))
		obj.Move(fyne.NewPos((containerSize.Width-l.width)/2, (containerSize.Height-l.width)/2))
	}
}

func (l *logoLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(l.width, l.width)
}

// ScreensaverMode determines screensaver operation mode
type ScreensaverMode int

const (
	ModeScreensaver ScreensaverMode = iota // Fullscreen screensaver
	ModeConfig                             // Configuration dialog
	ModePreview                            // Preview in Windows settings
)

func init() {
	runtime.LockOSThread() // OpenGL requires single-threaded execution
	rand.Seed(time.Now().UnixNano())

	// Load optional UI assets from repository `assets/` directory.
	// We keep screensaver runtime functional even when assets are absent.
	iconPNGData = readOptionalAsset("icon.png")
	iconICOData = readOptionalAsset("icon.ico")
	logoPNGData = readOptionalAsset("logo.png")
}

func readOptionalAsset(fileName string) []byte {
	candidates := []string{
		"../assets/" + fileName, // default when running from `source/`
		"assets/" + fileName,    // fallback when cwd is repo root
		"./assets/" + fileName,  // explicit local variant
	}
	for _, path := range candidates {
		data, err := os.ReadFile(path)
		if err == nil && len(data) > 0 {
			return data
		}
	}
	return nil
}

// parseColor parses hex color string into color.Color
func parseColor(hex string) color.Color {
	hex = strings.TrimPrefix(hex, "#")
	var r, g, b uint8
	if len(hex) == 6 {
		fmt.Sscanf(hex, "%02x%02x%02x", &r, &g, &b)
	}
	return color.RGBA{R: r, G: g, B: b, A: 255}
}

// preprocessJSON fixes common JSON issues like unescaped newlines in string literals
func preprocessJSON(data []byte) ([]byte, error) {
	// Convert to string for easier manipulation
	jsonStr := string(data)

	// Fix unescaped newlines in string literals
	// Pattern: find string literals (between quotes) and escape newlines inside them
	var result strings.Builder
	inString := false
	escapeNext := false

	for i := 0; i < len(jsonStr); i++ {
		char := jsonStr[i]

		if escapeNext {
			result.WriteByte(char)
			escapeNext = false
			continue
		}

		if char == '\\' {
			result.WriteByte(char)
			escapeNext = true
			continue
		}

		if char == '"' {
			// Check if this is an escaped quote or a real quote
			// Count backslashes before this quote
			backslashCount := 0
			for j := i - 1; j >= 0 && jsonStr[j] == '\\'; j-- {
				backslashCount++
			}
			// If even number of backslashes, this is a real quote
			if backslashCount%2 == 0 {
				inString = !inString
			}
			result.WriteByte(char)
			continue
		}

		if inString {
			// Inside string literal - escape newlines, tabs, and other control characters
			if char == '\n' {
				result.WriteString("\\n")
			} else if char == '\r' {
				result.WriteString("\\r")
			} else if char == '\t' {
				result.WriteString("\\t")
			} else if char < 0x20 {
				// Other control characters - escape as \uXXXX
				result.WriteString(fmt.Sprintf("\\u%04x", char))
			} else {
				result.WriteByte(char)
			}
		} else {
			result.WriteByte(char)
		}
	}

	return []byte(result.String()), nil
}

// loadEmbeddedShader loads and parses shader from embedded JSON file
func loadEmbeddedShader() (*ShaderData, error) {
	// Use embedded shader data
	data := shaderJSONData
	if len(data) == 0 {
		return nil, fmt.Errorf("embedded shader data is empty")
	}

	// Preprocess JSON to fix common issues (unescaped newlines, etc.)
	preprocessedData, err := preprocessJSON(data)
	if err != nil {
		return nil, fmt.Errorf("error preprocessing JSON: %v", err)
	}

	// Parse JSON
	var shaderData ShaderData
	if err := json.Unmarshal(preprocessedData, &shaderData); err != nil {
		return nil, fmt.Errorf("error parsing JSON: %v", err)
	}

	if len(shaderData.Passes) == 0 {
		return nil, fmt.Errorf("shader file contains no passes")
	}

	return &shaderData, nil
}

// removeComments removes all comments from shader code
func removeComments(code string) string {
	var result strings.Builder
	lines := strings.Split(code, "\n")
	inBlockComment := false

	for _, line := range lines {
		var processedLine strings.Builder
		i := 0
		for i < len(line) {
			if inBlockComment {
				// Look for end of block comment
				if i+1 < len(line) && line[i] == '*' && line[i+1] == '/' {
					inBlockComment = false
					i += 2
					continue
				}
				i++
				continue
			}

			// Check for block comment start
			if i+1 < len(line) && line[i] == '/' && line[i+1] == '*' {
				inBlockComment = true
				i += 2
				continue
			}

			// Check for line comment
			if i+1 < len(line) && line[i] == '/' && line[i+1] == '/' {
				// Rest of line is comment, stop processing this line
				break
			}

			processedLine.WriteByte(line[i])
			i++
		}

		// Only add line if it has content (after removing comments)
		trimmed := strings.TrimSpace(processedLine.String())
		if trimmed != "" || !inBlockComment {
			result.WriteString(processedLine.String())
			result.WriteString("\n")
		}
	}

	return result.String()
}

// determineVariableType determines the type of a variable based on its declaration chain or usage
func determineVariableType(varName string, code string, lines []string, lineIndex int) string {
	// First, check if variable is part of a multi-declaration chain
	// Look backwards to find the start of the chain where type is explicitly declared
	// Pattern: "vec2 r = ...," or "float i = ...," etc.
	typeDeclPattern := regexp.MustCompile(`\b(vec[234]|float|int|bool)\s+\w+\s*=`)

	for j := lineIndex - 1; j >= 0 && j >= lineIndex-20; j-- {
		prevLine := strings.TrimSpace(lines[j])
		if prevLine == "" {
			continue
		}

		// Check if this line is part of the chain (ends with comma)
		if !strings.HasSuffix(prevLine, ",") {
			// If line doesn't end with comma, check if it's the start of the chain
			// Look for explicit type declaration like "vec2 r = ..."
			if matches := typeDeclPattern.FindStringSubmatch(prevLine); matches != nil {
				varType := matches[1]
				// Return appropriate default value based on type
				switch varType {
				case "vec2":
					return "vec2(0.0)"
				case "vec3":
					return "vec3(0.0)"
				case "vec4":
					return "vec4(0.0)"
				case "float":
					return "0.0"
				case "int":
					return "0"
				case "bool":
					return "false"
				}
			}
			// If we hit a line that doesn't end with comma and isn't the start, we're out of the chain
			break
		}

		// Line ends with comma, check if it's the start of the chain with explicit type
		if matches := typeDeclPattern.FindStringSubmatch(prevLine); matches != nil {
			varType := matches[1]
			// Return appropriate default value based on type
			switch varType {
			case "vec2":
				return "vec2(0.0)"
			case "vec3":
				return "vec3(0.0)"
			case "vec4":
				return "vec4(0.0)"
			case "float":
				return "0.0"
			case "int":
				return "0"
			case "bool":
				return "false"
			}
		}
	}

	// Check usage patterns to determine type
	varNameDot := varName + "."

	// Check for component access that requires specific types
	if strings.Contains(code, varNameDot+"w") || strings.Contains(code, varName+".w") {
		// .w requires vec4
		return "vec4(0.0)"
	}
	if strings.Contains(code, varNameDot+"z") || strings.Contains(code, varName+".z") {
		// .z requires at least vec3
		return "vec4(0.0)"
	}

	// Check for swizzle patterns
	swizzlePattern := regexp.MustCompile(regexp.QuoteMeta(varName) + `\.([xyzw]{2,4})`)
	if matches := swizzlePattern.FindAllString(code, -1); len(matches) > 0 {
		// Variable is used with swizzle, likely vec2 or vec4
		// Check if used in accumulation
		if strings.Contains(code, varName+" +=") || strings.Contains(code, varName+" =") {
			// Default to vec2 for accumulation (common in fullscreen shaders)
			return "vec2(0.0)"
		}
		return "vec2(0.0)"
	}

	// Check for component access .x or .y
	if strings.Contains(code, varNameDot+"x") || strings.Contains(code, varNameDot+"y") ||
		strings.Contains(code, varName+".x") || strings.Contains(code, varName+".y") {
		// Could be vec2, vec3, or vec4
		// Check if used in accumulation
		if strings.Contains(code, varName+" +=") || strings.Contains(code, varName+" =") {
			return "vec2(0.0)"
		}
		return "vec2(0.0)"
	}

	// Check for arithmetic operations
	if strings.Contains(code, varName+" +=") || strings.Contains(code, varName+" =") ||
		strings.Contains(code, varName+" -=") || strings.Contains(code, varName+" *=") ||
		strings.Contains(code, varName+" /=") {
		// Used in accumulation/assignment, likely vec2 or vec4
		// Default to vec2 (more common in fullscreen shaders)
		return "vec2(0.0)"
	}

	// Check if variable is used in expressions
	if strings.Contains(code, varName+" ") || strings.Contains(code, varName+"(") ||
		strings.Contains(code, varName+")") || strings.Contains(code, "("+varName) {
		// Variable is used but type is unclear, default to vec2
		return "vec2(0.0)"
	}

	// Default to vec2 (most common case in this shader family)
	return "vec2(0.0)"
}

// removeOrphanedAssignments removes assignments that reference undeclared variables
// Example: "vec2 p = bpos.zx;" where bpos is not declared
// BUT: It should NOT remove lines with type declarations like "vec2 dg = tri2(bp*1.85)*.75;"
// because these are new variable declarations, not orphaned assignments
func removeOrphanedAssignments(code string) string {
	lines := strings.Split(code, "\n")
	var filteredLines []string

	for i, line := range lines {
		// Check for assignment pattern WITHOUT type declaration: "varName = expression;" (no type before varName)
		// This is an orphaned assignment - assignment without declaration
		orphanedPattern := regexp.MustCompile(`^\s*([a-zA-Z_][a-zA-Z0-9_]*)\s*=\s*([^;]+);`)
		if matches := orphanedPattern.FindStringSubmatch(line); matches != nil {
			varName := matches[1]
			expression := matches[2]

			// Skip if this line has a type declaration (e.g., "vec2 dg = ..." is NOT orphaned)
			// Check if line starts with a type keyword
			typePattern := regexp.MustCompile(`^\s*(vec[234]|float|int|bool|mat[234])\s+`)
			if typePattern.MatchString(line) {
				// This is a type declaration, not an orphaned assignment - keep it
				filteredLines = append(filteredLines, line)
				continue
			}

			// Skip reserved keywords
			if varName == "if" || varName == "for" || varName == "while" || varName == "return" {
				filteredLines = append(filteredLines, line)
				continue
			}

			// Check if variable is a function parameter (e.g., fragColor in mainImage)
			// Look for function definitions that contain this variable as a parameter
			beforeCode := strings.Join(lines[:i], "\n")
			paramPattern := regexp.MustCompile(`\b(out|in|inout)\s+(vec[234]|float|int|bool|mat[234])\s+` + regexp.QuoteMeta(varName) + `\s*[,)]`)
			if paramPattern.MatchString(beforeCode) {
				// Variable is a function parameter - keep it
				filteredLines = append(filteredLines, line)
				continue
			}

			// Check if variable is declared before this line
			declPattern := regexp.MustCompile(`\b(vec[234]|float|int|bool|mat[234])\s+` + regexp.QuoteMeta(varName) + `\s*[=;]`)
			if !declPattern.MatchString(beforeCode) {
				// Check if expression references undeclared variables
				varRefPattern := regexp.MustCompile(`\b([a-zA-Z_][a-zA-Z0-9_]*)`)
				varRefs := varRefPattern.FindAllString(expression, -1)

				// Check if any referenced variable is not declared
				isOrphaned := false
				for _, ref := range varRefs {
					// Skip built-in functions and constants
					if ref == "vec2" || ref == "vec3" || ref == "vec4" || ref == "sin" || ref == "cos" ||
						ref == "abs" || ref == "fract" || ref == "clamp" || ref == "pow" || ref == "mix" ||
						ref == "smoothstep" || ref == "exp2" || ref == "normalize" || ref == "dot" ||
						ref == "length" || ref == "floor" || ref == "step" || ref == "iTime" || ref == "iResolution" ||
						ref == "gl_FragCoord" || ref == "x" || ref == "y" || ref == "z" || ref == "w" ||
						ref == "r" || ref == "g" || ref == "b" || ref == "a" || ref == "xy" || ref == "zx" ||
						ref == "rgb" || ref == "xyyx" || ref == varName || ref == "time" || ref == "spd" ||
						ref == "mm2" || ref == "tri2" || ref == "tri" || ref == "m2" || ref == "bp" || ref == "p" {
						continue
					}

					// Check if variable is declared before this line
					refDeclPattern := regexp.MustCompile(`\b(vec[234]|float|int|bool|mat[234])\s+` + regexp.QuoteMeta(ref) + `\s*[=;]`)
					// Also check if it's a function parameter
					refParamPattern := regexp.MustCompile(`\b(out|in|inout)\s+(vec[234]|float|int|bool|mat[234])\s+` + regexp.QuoteMeta(ref) + `\s*[,)]`)
					if !refDeclPattern.MatchString(beforeCode) && !refParamPattern.MatchString(beforeCode) {
						// Variable is not declared - this is an orphaned assignment
						isOrphaned = true
						break
					}
				}

				if isOrphaned {
					// Remove this line
					continue
				}
			}
		}
		filteredLines = append(filteredLines, line)
	}

	return strings.Join(filteredLines, "\n")
}

// fixMainImageFragColor removes duplicate fragColor declaration in mainImage
// mainImage already has "out vec4 fragColor" as parameter, so we shouldn't redeclare it
func fixMainImageFragColor(code string) string {
	lines := strings.Split(code, "\n")

	// Find mainImage function
	mainImageStart := -1
	for i, line := range lines {
		if strings.Contains(strings.TrimSpace(line), "void mainImage") {
			mainImageStart = i
			break
		}
	}

	if mainImageStart == -1 {
		return code // mainImage not found
	}

	// Find mainImage function end
	braceCount := 0
	mainImageEnd := len(lines)
	for i := mainImageStart; i < len(lines); i++ {
		line := lines[i]
		braceCount += strings.Count(line, "{") - strings.Count(line, "}")
		if braceCount == 0 && i > mainImageStart {
			mainImageEnd = i + 1
			break
		}
	}

	// Look for duplicate fragColor declaration inside mainImage
	for i := mainImageStart; i < mainImageEnd; i++ {
		trimmed := strings.TrimSpace(lines[i])
		// Check for "vec4 fragColor = ..." (not "out vec4 fragColor" which is parameter)
		if strings.Contains(trimmed, "vec4 fragColor =") || strings.Contains(trimmed, "vec4 fragColor=") {
			// Replace with just assignment: "fragColor = ..."
			// Extract assignment part
			if idx := strings.Index(trimmed, "fragColor"); idx >= 0 {
				assignment := trimmed[idx:]
				lines[i] = strings.Repeat(" ", len(lines[i])-len(strings.TrimLeft(lines[i], " \t"))) + assignment
			}
		}
	}

	return strings.Join(lines, "\n")
}

// findFunctionScope finds which function a line belongs to
// Returns: line index of function start, true if in mainImage
func findFunctionScope(lines []string, lineIndex int) (int, bool) {
	// Look backwards to find function definition
	for i := lineIndex; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		// Check for function definition
		if strings.Contains(line, "void ") ||
			(strings.Contains(line, "float ") && strings.Contains(line, "(")) ||
			(strings.Contains(line, "vec") && strings.Contains(line, "(")) {
			// Check if it's mainImage
			if strings.Contains(line, "mainImage") {
				return i, true
			}
			// It's another function
			return i, false
		}
	}
	return -1, false
}

// isVariableDeclaredInScope checks if a variable is declared in a specific scope
func isVariableDeclaredInScope(code string, varName string, scopeStart int, scopeEnd int) bool {
	// Check for type declaration: "vec2 varName", "float varName", etc.
	declPattern := regexp.MustCompile(`\b(vec[234]|float|int|bool)\s+` + regexp.QuoteMeta(varName) + `\s*[=;]`)
	scopeCode := code[scopeStart:scopeEnd]
	return declPattern.MatchString(scopeCode)
}

func fixShaderCode(code string) string {
	// First, remove comments to make parsing easier
	code = removeComments(code)

	// Fix uninitialized variables that are used in loops or expressions
	// Common patterns:
	// 1. ", varName;" in multi-declaration chain
	// 2. standalone "varName;" on its own line
	// 3. Type declarations without initialization like "vec4 varName;" or "float a;"

	lines := strings.Split(code, "\n")

	// Track variables that are declared but not initialized
	uninitializedVars := make(map[string]string) // var name -> default value

	// Pattern 1: Variables in multi-declaration chains (e.g., ", w;", ", x;", ", y;")
	// Match pattern: ", variableName;" where variableName is any identifier
	chainVarPattern := regexp.MustCompile(`,\s+(\w+)\s*;`)

	// Pattern 2: Standalone variable declarations (e.g., "w;", "x;", "y;")
	// Match pattern: variableName; (with optional leading whitespace)
	standaloneVarPattern := regexp.MustCompile(`^\s*(\w+)\s*;`)

	// First pass: find and fix uninitialized variable declarations
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		// Pattern 1: ", varName;" in multi-declaration on same line
		if matches := chainVarPattern.FindStringSubmatch(line); matches != nil {
			varName := matches[1]
			// Skip if variable is already initialized
			if strings.Contains(line, varName+" =") {
				continue
			}
			// First, try to extract type from the same line (e.g., "float i = .2, a;")
			varType := ""
			typeDeclPattern := regexp.MustCompile(`\b(vec[234]|float|int|bool)\s+\w+`)
			if typeMatch := typeDeclPattern.FindStringSubmatch(line); typeMatch != nil {
				// Type found in the same line, use it
				switch typeMatch[1] {
				case "vec2":
					varType = "vec2(0.0)"
				case "vec3":
					varType = "vec3(0.0)"
				case "vec4":
					varType = "vec4(0.0)"
				case "float":
					varType = "0.0"
				case "int":
					varType = "0"
				case "bool":
					varType = "false"
				}
			}
			// If type not found in same line, look in previous lines (chain across lines)
			if varType == "" {
				varType = determineVariableType(varName, code, lines, i)
			}
			// Replace ", varName;" with ", varName = <type>;"
			lines[i] = strings.Replace(line, ", "+varName+";", ", "+varName+" = "+varType+";", 1)
			uninitializedVars[varName] = varType
			continue
		}

		// Pattern 2: standalone "varName;" on its own line (may be part of multi-declaration chain)
		if matches := standaloneVarPattern.FindStringSubmatch(line); matches != nil {
			varName := matches[1]
			// Skip reserved keywords and already initialized variables
			if varName == "if" || varName == "for" || varName == "while" || varName == "return" ||
				strings.Contains(line, varName+" =") {
				continue
			}

			// Check function scope to avoid initializing variables in wrong scope
			funcStart, isMainImage := findFunctionScope(lines, i)

			// If we're inside a function other than mainImage
			if !isMainImage && funcStart >= 0 {
				// Check if variable is declared in mainImage
				// Find mainImage function
				mainImageStart := -1
				for j := 0; j < len(lines); j++ {
					if strings.Contains(strings.TrimSpace(lines[j]), "mainImage") {
						mainImageStart = j
						break
					}
				}

				if mainImageStart >= 0 {
					// Check if variable is declared in mainImage
					mainImageCode := strings.Join(lines[mainImageStart:], "\n")
					declPattern := regexp.MustCompile(`\b(vec[234]|float|int|bool)\s+` + regexp.QuoteMeta(varName) + `\s*[=;]`)
					if declPattern.MatchString(mainImageCode) {
						// Variable is declared in mainImage, don't initialize it here
						// It should be initialized in mainImage, not in this function
						continue
					}
				}
			}

			// Check if variable is used in the code (not just declared)
			// But first check if it's declared elsewhere (in mainImage or globally)
			// If it's declared elsewhere, don't initialize it here
			varIsDeclaredElsewhere := false

			// Check if variable is declared in mainImage
			mainImageStart := -1
			for j := 0; j < len(lines); j++ {
				if strings.Contains(strings.TrimSpace(lines[j]), "mainImage") {
					mainImageStart = j
					break
				}
			}

			if mainImageStart >= 0 {
				mainImageCode := strings.Join(lines[mainImageStart:], "\n")
				declPattern := regexp.MustCompile(`\b(vec[234]|float|int|bool)\s+` + regexp.QuoteMeta(varName) + `\s*[=;]`)
				if declPattern.MatchString(mainImageCode) {
					varIsDeclaredElsewhere = true
				}
			}

			// Also check if declared globally (before any function)
			if !varIsDeclaredElsewhere {
				// Find first function
				firstFuncLine := -1
				for j := 0; j < i; j++ {
					trimmedLine := strings.TrimSpace(lines[j])
					if strings.Contains(trimmedLine, "void ") ||
						(strings.Contains(trimmedLine, "float ") && strings.Contains(trimmedLine, "(")) ||
						(strings.Contains(trimmedLine, "vec") && strings.Contains(trimmedLine, "(")) {
						firstFuncLine = j
						break
					}
				}

				if firstFuncLine >= 0 {
					globalCode := strings.Join(lines[:firstFuncLine], "\n")
					declPattern := regexp.MustCompile(`\b(vec[234]|float|int|bool)\s+` + regexp.QuoteMeta(varName) + `\s*[=;]`)
					if declPattern.MatchString(globalCode) {
						varIsDeclaredElsewhere = true
					}
				}
			}

			// If variable is declared elsewhere, don't initialize it here
			if varIsDeclaredElsewhere {
				continue
			}

			varIsUsed := strings.Contains(code, varName+" ") || strings.Contains(code, varName+".") ||
				strings.Contains(code, varName+"+") || strings.Contains(code, varName+"-") ||
				strings.Contains(code, varName+"*") || strings.Contains(code, varName+"/") ||
				strings.Contains(code, varName+"=") || strings.Contains(code, "("+varName) ||
				strings.Contains(code, varName+")")

			if varIsUsed {
				// Determine type based on usage and context
				varType := determineVariableType(varName, code, lines, i)
				// Replace "varName;" with "varName = <type>;" keeping original indentation
				indent := ""
				for k := 0; k < len(line) && (line[k] == ' ' || line[k] == '\t'); k++ {
					indent += string(line[k])
				}
				lines[i] = indent + varName + " = " + varType + ";"
				uninitializedVars[varName] = varType
			}
			continue
		}

		// Pattern 3: type declarations without initialization
		// Match patterns like "vec4 w;" or "float a;" (but not "vec4 w = ...;")
		// Use regex to find type declarations
		declPattern := regexp.MustCompile(`\b(vec[234]|float|int|bool)\s+(\w+)\s*;`)
		if matches := declPattern.FindStringSubmatch(trimmed); matches != nil {
			varType := matches[1]
			varName := matches[2]

			// Skip if variable is already initialized (has "=" in declaration)
			if strings.Contains(trimmed, varName+" =") {
				continue
			}

			// Check if we're inside a function other than mainImage
			funcStart, isMainImage := findFunctionScope(lines, i)
			if !isMainImage && funcStart >= 0 {
				// Check if variable is declared in mainImage or globally
				// If it's declared elsewhere, don't initialize it here
				varIsDeclaredElsewhere := false

				// Check mainImage
				mainImageStart := -1
				for j := 0; j < len(lines); j++ {
					if strings.Contains(strings.TrimSpace(lines[j]), "mainImage") {
						mainImageStart = j
						break
					}
				}

				if mainImageStart >= 0 {
					mainImageCode := strings.Join(lines[mainImageStart:], "\n")
					declPattern := regexp.MustCompile(`\b(vec[234]|float|int|bool)\s+` + regexp.QuoteMeta(varName) + `\s*[=;]`)
					if declPattern.MatchString(mainImageCode) {
						varIsDeclaredElsewhere = true
					}
				}

				// Check global scope (before first function)
				if !varIsDeclaredElsewhere && funcStart >= 0 {
					globalCode := strings.Join(lines[:funcStart], "\n")
					declPattern := regexp.MustCompile(`\b(vec[234]|float|int|bool)\s+` + regexp.QuoteMeta(varName) + `\s*[=;]`)
					if declPattern.MatchString(globalCode) {
						varIsDeclaredElsewhere = true
					}
				}

				// If variable is declared elsewhere, don't initialize it here
				if varIsDeclaredElsewhere {
					continue
				}
			}

			// Check if variable is used later in code
			remainingCode := strings.Join(lines[i+1:], "\n")
			isUsed := strings.Contains(remainingCode, varName+" ") ||
				strings.Contains(remainingCode, varName+".") ||
				strings.Contains(remainingCode, varName+"+") ||
				strings.Contains(remainingCode, varName+"-") ||
				strings.Contains(remainingCode, varName+"*") ||
				strings.Contains(remainingCode, varName+"/") ||
				strings.Contains(remainingCode, varName+"=") ||
				strings.Contains(remainingCode, "("+varName) ||
				strings.Contains(remainingCode, varName+")")

			if isUsed {
				// Determine default value based on type
				var defaultValue string
				switch varType {
				case "vec2":
					defaultValue = "vec2(0.0)"
				case "vec3":
					defaultValue = "vec3(0.0)"
				case "vec4":
					defaultValue = "vec4(0.0)"
				case "float":
					defaultValue = "0.0"
				case "int":
					defaultValue = "0"
				case "bool":
					defaultValue = "false"
				default:
					defaultValue = "0.0"
				}
				uninitializedVars[varName] = defaultValue
				// Initialize the variable
				lines[i] = strings.Replace(trimmed, varName+";", varName+" = "+defaultValue+";", 1)
			}
		}
	}

	code = strings.Join(lines, "\n")

	// Additional pass: find and fix assignments without declarations (e.g., "col = vec3(0.0);" without "vec3 col;")
	// This handles cases where fixShaderCode added assignment but variable wasn't declared
	lines = strings.Split(code, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		// Pattern: "varName = value;" without type declaration
		// Match: identifier followed by = but no type declaration before
		assignPattern := regexp.MustCompile(`^\s*(\w+)\s*=\s*([^;]+);`)
		if matches := assignPattern.FindStringSubmatch(line); matches != nil {
			varName := matches[1]
			// Skip if it's a function call or reserved keyword
			if varName == "if" || varName == "for" || varName == "while" || varName == "return" {
				continue
			}

			// Check if variable is declared before this line
			beforeCode := strings.Join(lines[:i], "\n")
			declPattern := regexp.MustCompile(`\b(vec[234]|float|int|bool)\s+` + regexp.QuoteMeta(varName) + `\s*[=;]`)
			if !declPattern.MatchString(beforeCode) {
				// Variable is not declared, check if we're in a function other than mainImage
				funcStart, isMainImage := findFunctionScope(lines, i)
				if !isMainImage && funcStart >= 0 {
					// Check if variable is declared in mainImage
					mainImageStart := -1
					for j := 0; j < len(lines); j++ {
						if strings.Contains(strings.TrimSpace(lines[j]), "mainImage") {
							mainImageStart = j
							break
						}
					}

					if mainImageStart >= 0 {
						mainImageCode := strings.Join(lines[mainImageStart:], "\n")
						if declPattern.MatchString(mainImageCode) {
							// Variable is declared in mainImage, remove this assignment
							// It shouldn't be assigned here
							lines[i] = "" // Remove the line
							continue
						}
					}
					// Variable is not declared anywhere, we need to declare it
					// Determine type from the assignment value
					assignValue := matches[2]
					var varType string
					if strings.Contains(assignValue, "vec2(") {
						varType = "vec2"
					} else if strings.Contains(assignValue, "vec3(") {
						varType = "vec3"
					} else if strings.Contains(assignValue, "vec4(") {
						varType = "vec4"
					} else if strings.Contains(assignValue, ".") && !strings.Contains(assignValue, "(") {
						// Float literal
						varType = "float"
					} else {
						varType = "float" // Default
					}

					// Add declaration before assignment
					indent := ""
					for k := 0; k < len(line) && (line[k] == ' ' || line[k] == '\t'); k++ {
						indent += string(line[k])
					}
					lines[i] = indent + varType + " " + varName + " = " + assignValue + ";"
				}
			}
		}
	}
	code = strings.Join(lines, "\n")
	// Remove empty lines
	lines = strings.Split(code, "\n")
	var filteredLines []string
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			filteredLines = append(filteredLines, line)
		}
	}
	code = strings.Join(filteredLines, "\n")

	// Second pass: catch any remaining uninitialized variables that might have been missed
	// Look for patterns like "varName;" that weren't caught in first pass
	lines = strings.Split(code, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		// Check for standalone variable declarations that might have been missed
		standaloneMatch := standaloneVarPattern.FindStringSubmatch(line)
		if standaloneMatch != nil {
			varName := standaloneMatch[1]
			// Skip if already initialized or reserved keywords
			if strings.Contains(line, varName+" =") || varName == "if" || varName == "for" ||
				varName == "while" || varName == "return" {
				continue
			}

			// Check if variable is used but not initialized
			if strings.Contains(code, varName+" ") || strings.Contains(code, varName+".") ||
				strings.Contains(code, varName+"+") || strings.Contains(code, varName+"-") ||
				strings.Contains(code, varName+"*") || strings.Contains(code, varName+"/") ||
				strings.Contains(code, varName+"=") {
				// Check if it's not already in our map
				if _, exists := uninitializedVars[varName]; !exists {
					// Check if variable is actually uninitialized
					if !strings.Contains(code, varName+" =") && !strings.Contains(code, varName+"=") {
						varType := determineVariableType(varName, code, lines, i)
						indent := ""
						for k := 0; k < len(line) && (line[k] == ' ' || line[k] == '\t'); k++ {
							indent += string(line[k])
						}
						lines[i] = indent + varName + " = " + varType + ";"
						uninitializedVars[varName] = varType
					}
				}
			}
		}
	}

	code = strings.Join(lines, "\n")

	// Remove orphaned assignments (assignments without declarations that reference undeclared variables)
	// Example: "vec2 p = bpos.zx;" where bpos is not declared
	code = removeOrphanedAssignments(code)

	// Fix mainImage function - remove duplicate fragColor declaration
	code = fixMainImageFragColor(code)

	// Second pass: ensure variables are initialized before use in loops
	// This handles cases where variable is declared but used in loop before initialization
	if strings.Contains(code, "for(") {
		// Find all for loops
		loopPattern := regexp.MustCompile(`for\s*\([^)]*\)`)
		loopMatches := loopPattern.FindAllStringIndex(code, -1)

		// Process loops in reverse order to avoid index shifting
		for idx := len(loopMatches) - 1; idx >= 0; idx-- {
			match := loopMatches[idx]
			loopStart := match[0]
			loopEnd := match[1]

			beforeLoop := code[:loopStart]
			loopBody := code[loopEnd:]

			// Find the opening brace of the loop body
			braceIdx := strings.Index(loopBody, "{")
			if braceIdx == -1 {
				continue
			}

			loopBodyStart := loopEnd + braceIdx
			loopBodyCode := code[loopBodyStart:]

			// Check each uninitialized variable
			for varName, defaultValue := range uninitializedVars {
				// Check if variable is used in loop body
				if strings.Contains(loopBodyCode, varName+" ") ||
					strings.Contains(loopBodyCode, varName+".") ||
					strings.Contains(loopBodyCode, varName+"+") ||
					strings.Contains(loopBodyCode, varName+"-") ||
					strings.Contains(loopBodyCode, varName+"*") ||
					strings.Contains(loopBodyCode, varName+"/") ||
					strings.Contains(loopBodyCode, varName+"=") ||
					strings.Contains(loopBodyCode, "("+varName) {
					// Check if variable is initialized before loop
					if !strings.Contains(beforeLoop, varName+" =") &&
						!strings.Contains(beforeLoop, varName+"=") {
						// Insert initialization right before loop
						indent := "    "
						code = code[:loopStart] + indent + varName + " = " + defaultValue + ";\n" + code[loopStart:]
					}
				}
			}
		}
	}

	return code
}

// getMainShaderCode extracts main shader code from parsed shader data
// Returns vertex and fragment shader code
func getMainShaderCode(shaderData *ShaderData) (string, string, error) {
	// Look for "image" type pass or use first pass
	var mainPass *ShaderPass
	for i := range shaderData.Passes {
		if shaderData.Passes[i].Type == "image" || shaderData.Passes[i].Name == "Image" {
			mainPass = &shaderData.Passes[i]
			break
		}
	}

	// If not found, use first pass
	if mainPass == nil {
		mainPass = &shaderData.Passes[0]
	}

	// Fix common shader issues: initialize uninitialized variables
	shaderCode := fixShaderCode(mainPass.Code)

	// Debug: output processed shader code if debug mode is enabled
	if DEBUG_MODE {
		log.Printf("Processed shader code length: %d bytes", len(shaderCode))
		log.Printf("\n=== PROCESSED SHADER CODE (after removing comments and initializing variables) ===\n%s\n=== END OF PROCESSED SHADER CODE ===\n", shaderCode)
	}

	// Base vertex shader for fullscreen quad rendering.
	vertexShader := `#version 330 core
layout(location = 0) in vec2 aPos;
layout(location = 1) in vec2 aTexCoord;
out vec2 fragCoord;

void main() {
    fragCoord = aTexCoord;
    gl_Position = vec4(aPos * 2.0 - 1.0, 0.0, 1.0);
}` + "\x00"

	// Fragment shader from shader JSON.
	// The shader entrypoint uses mainImage(out vec4 fragColor, in vec2 fragCoord)
	// where fragCoord is pixel coordinates in screen space [0...iResolution.xy]
	fragmentShaderTemplate := `#version 330 core
in vec2 fragCoord;
out vec4 fragColor;

uniform vec3 iResolution;
uniform float iTime;
uniform float iTimeDelta;
uniform int iFrame;
uniform float iFrameRate;
uniform vec4 iMouse;
uniform vec4 iDate;
uniform float iSampleRate;
uniform vec3 iChannelResolution[4];
uniform float iChannelTime[4];

uniform sampler2D iChannel0;
uniform sampler2D iChannel1;
uniform sampler2D iChannel2;
uniform sampler2D iChannel3;
uniform float iFade;

` + shaderCode + `

void main() {
    vec2 fragCoordScreen = fragCoord * iResolution.xy;
    mainImage(fragColor, fragCoordScreen);
    fragColor.rgb *= iFade;
}` + "\x00"

	// Remove comments from wrapper before compilation
	fragmentShader := removeComments(fragmentShaderTemplate)

	return vertexShader, fragmentShader, nil
}

// styledButton - custom button with specified colors
type styledButton struct {
	widget.BaseWidget
	text      string
	textColor color.Color
	bgColor   color.Color
	onTapped  func()
}

func newStyledButton(text string, textColor, bgColor color.Color, onTapped func()) *styledButton {
	b := &styledButton{
		text:      text,
		textColor: textColor,
		bgColor:   bgColor,
		onTapped:  onTapped,
	}
	b.ExtendBaseWidget(b)
	return b
}

func (b *styledButton) CreateRenderer() fyne.WidgetRenderer {
	rect := canvas.NewRectangle(b.bgColor)
	rect.SetMinSize(fyne.NewSize(150, 35))

	textObj := canvas.NewText(b.text, b.textColor)
	textObj.Alignment = fyne.TextAlignCenter
	textObj.TextSize = 14

	content := container.NewStack(
		rect,
		container.NewCenter(textObj),
	)

	return &styledButtonRenderer{
		button:  b,
		rect:    rect,
		textObj: textObj,
		content: content,
	}
}

func (b *styledButton) Tapped(*fyne.PointEvent) {
	if b.onTapped != nil {
		b.onTapped()
	}
}

type styledButtonRenderer struct {
	button  *styledButton
	rect    *canvas.Rectangle
	textObj *canvas.Text
	content fyne.CanvasObject
}

func (r *styledButtonRenderer) Layout(size fyne.Size) {
	r.content.Resize(size)
}

func (r *styledButtonRenderer) MinSize() fyne.Size {
	return r.content.MinSize()
}

func (r *styledButtonRenderer) Refresh() {
	r.rect.FillColor = r.button.bgColor
	r.textObj.Color = r.button.textColor
	r.textObj.Text = r.button.text
}

func (r *styledButtonRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.content}
}

func (r *styledButtonRenderer) Destroy() {}

// detectScreensaverMode determines operation mode from command line arguments
// Windows screensaver arguments:
//   - /s or no arguments = screensaver mode (fullscreen)
//   - /c = configuration mode
//   - /p <HWND> = preview mode
func detectScreensaverMode() (ScreensaverMode, uintptr) {
	args := os.Args[1:]

	if len(args) == 0 {
		return ModeScreensaver, 0
	}

	for i, arg := range args {
		argLower := strings.ToLower(arg)
		switch {
		case argLower == "/s":
			return ModeScreensaver, 0
		case argLower == "/c" || strings.HasPrefix(argLower, "/c:"):
			// Configuration mode: /c or /c:15740 (with HWND after colon)
			return ModeConfig, 0
		case argLower == "/p" || strings.HasPrefix(argLower, "/p:"):
			// Preview mode with parent window HWND
			// Can be: /p <HWND> or /p:<HWND>
			var hwnd uintptr
			if strings.HasPrefix(argLower, "/p:") {
				// Extract HWND from /p:12345 format
				hwndStr := argLower[3:] // Skip "/p:"
				if parsedHWND, err := strconv.ParseUint(hwndStr, 10, 64); err == nil {
					hwnd = uintptr(parsedHWND)
				}
			} else if i+1 < len(args) {
				// Extract HWND from next argument /p 12345
				if parsedHWND, err := strconv.ParseUint(args[i+1], 10, 64); err == nil {
					hwnd = uintptr(parsedHWND)
				}
			}
			if hwnd != 0 {
				return ModePreview, hwnd
			}
			return ModePreview, 0
		}
	}

	// Default - screensaver mode
	return ModeScreensaver, 0
}

// runConfigMode starts configuration dialog
func runConfigMode() {
	myApp := app.New()
	// Note: Application icon will be set before creating window (see below)

	// Set application icon first (before creating window)
	// This ensures the icon is available for the window
	var appIconResource fyne.Resource
	if len(iconPNGData) > 0 {
		appIconResource = fyne.NewStaticResource("icon.png", iconPNGData)
		myApp.SetIcon(appIconResource)
	}

	// Build window title with command line arguments in debug mode
	windowTitle := CONFIG_WINDOW_TITLE
	if DEBUG_MODE {
		// Only show command line arguments (skip program name/path)
		if len(os.Args) > 1 {
			argsStr := strings.Join(os.Args[1:], " ")
			windowTitle = fmt.Sprintf("[Args: %s]", argsStr)
		} else {
			windowTitle = "[Args: (none)]"
		}
	}

	configWindow := myApp.NewWindow(windowTitle)
	windowWidth := float32(400)
	windowHeight := float32(300)
	configWindow.Resize(fyne.NewSize(windowWidth, windowHeight))
	configWindow.SetFixedSize(true) // Make window non-resizable
	// Note: Removing minimize/maximize buttons requires platform-specific code
	// and is not directly supported through Fyne API
	configWindow.CenterOnScreen()

	// Set window icon (use the same icon resource as application)
	if appIconResource != nil {
		configWindow.SetIcon(appIconResource)
	} else {
		// Set icon from embedded data
		if len(iconPNGData) > 0 {
			iconResource := fyne.NewStaticResource("icon.png", iconPNGData)
			configWindow.SetIcon(iconResource)
		}
	}

	// Parse colors from constants
	aboutTextColor := parseColor(ABOUT_TEXT_COLOR)
	infoTextColor := parseColor(INFO_TEXT_COLOR)
	windowBgColor := parseColor(WINDOW_BACKGROUND_COLOR)
	// Note: BUTTON_TEXT_COLOR and BUTTON_BACKGROUND_COLOR are defined in constants,
	// but button uses standard OS design

	// Create UI elements
	titleLabel := widget.NewLabel(SCREENSAVER_NAME)
	titleLabel.TextStyle = fyne.TextStyle{Bold: true}
	titleLabel.Alignment = fyne.TextAlignCenter

	// Load and scale logo
	// Calculate maximum logo size to fit everything in 300px height
	// 300px - 15 (top) - ~25 (label) - 15 (spacing) - 15 (spacing) - ~35 (button) - 15 (bottom) = ~180px
	var logoImage fyne.CanvasObject
	maxLogoSize := windowHeight - 15 - 25 - 15 - 15 - 35 - 15 // ~180px
	logoWidth := windowWidth / 2                              // 200px
	if logoWidth > maxLogoSize {
		logoWidth = maxLogoSize // Use smaller size if needed
	}
	// Load logo from embedded data
	if len(logoPNGData) > 0 {
		logoResource := fyne.NewStaticResource("logo.png", logoPNGData)
		logoIcon := widget.NewIcon(logoResource)
		// Use container with fixed width for scaling
		logoImage = container.New(&logoLayout{width: logoWidth}, logoIcon)
	} else {
		// If logo not found, create empty widget
		logoImage = widget.NewIcon(nil)
	}

	// Create title text with specified color, font size, and underline
	aboutText := canvas.NewText(SCREENSAVER_NAME, aboutTextColor)
	aboutText.Alignment = fyne.TextAlignCenter
	aboutText.TextSize = float32(ABOUT_TEXT_FONT_SIZE)
	aboutText.TextStyle = fyne.TextStyle{Underline: true}
	aboutLabel := container.NewCenter(aboutText)

	// Create info text lines (copyright, website, email) in blue color
	copyrightText := canvas.NewText(COPYRIGHT_TEXT, infoTextColor)
	copyrightText.Alignment = fyne.TextAlignCenter
	copyrightText.TextSize = float32(ABOUT_TEXT_FONT_SIZE)
	copyrightLabel := container.NewCenter(copyrightText)

	websiteText := canvas.NewText(WEBSITE_TEXT, infoTextColor)
	websiteText.Alignment = fyne.TextAlignCenter
	websiteText.TextSize = float32(ABOUT_TEXT_FONT_SIZE)
	websiteLabel := container.NewCenter(websiteText)

	emailText := canvas.NewText(EMAIL_TEXT, infoTextColor)
	emailText.Alignment = fyne.TextAlignCenter
	emailText.TextSize = float32(ABOUT_TEXT_FONT_SIZE)
	emailLabel := container.NewCenter(emailText)

	// Button to open website (use standard OS design)
	visitButton := widget.NewButton(VISIT_WEBSITE_BUTTON_TEXT, func() {
		// Open URL in browser using platform-specific function
		if err := openURL(WEBSITE_URL); err != nil {
			log.Printf("Error opening URL: %v", err)
		}
	})

	// Use custom layout for precise position control
	// Structure: 15px padding, title, 15px, logo, 15px, copyright, 5px, website, 5px, email, 15px, button, 15px padding
	allElements := []fyne.CanvasObject{
		aboutLabel,
		logoImage,
		copyrightLabel,
		websiteLabel,
		emailLabel,
		visitButton,
	}

	// Use equal spacing: topPadding and spacing between title and logo should be equal
	// topPadding is the space from top of window to top of title
	// spacing is the space from bottom of title to top of logo
	// To make them visually equal, reduce topPadding slightly
	spacingBetweenElements := float32(15) // Spacing between title and logo
	topPaddingValue := float32(12)        // Slightly less to compensate for visual perception
	content := container.New(&dialogLayout{
		width:         windowWidth,
		height:        windowHeight,
		topPadding:    topPaddingValue,
		bottomPadding: 15,
		spacing:       spacingBetweenElements,
	}, allElements...)

	// Create window background with specified color
	background := canvas.NewRectangle(windowBgColor)
	background.Resize(fyne.NewSize(windowWidth, windowHeight))

	// Wrap content in container with background
	windowContent := container.NewStack(background, content)

	// Set content - window will be exactly 400x300
	configWindow.SetContent(windowContent)
	// Force window size after setting content
	configWindow.Resize(fyne.NewSize(windowWidth, windowHeight))
	configWindow.ShowAndRun()
}

// runPreviewMode starts preview mode
func runPreviewMode(parentHWND uintptr) {
	// For preview create small window with OpenGL
	if err := glfw.Init(); err != nil {
		log.Fatalln("Error initializing GLFW:", err)
	}
	defer glfw.Terminate()

	glfw.WindowHint(glfw.Resizable, glfw.False)
	glfw.WindowHint(glfw.ContextVersionMajor, 3)
	glfw.WindowHint(glfw.ContextVersionMinor, 3)
	glfw.WindowHint(glfw.OpenGLProfile, glfw.OpenGLCoreProfile)
	glfw.WindowHint(glfw.OpenGLForwardCompatible, glfw.True)

	// Build window title with command line arguments in debug mode
	windowTitle := SCREENSAVER_NAME
	if DEBUG_MODE {
		// Only show command line arguments (skip program name/path)
		if len(os.Args) > 1 {
			argsStr := strings.Join(os.Args[1:], " ")
			windowTitle = fmt.Sprintf("[Args: %s]", argsStr)
		} else {
			windowTitle = "[Args: (none)]"
		}
	}

	// Determine preview window size
	// If parent HWND is provided, we'll get the size from parent window
	// Otherwise use default size
	previewWidth, previewHeight := 320, 240 // Default preview size

	// If parent HWND is provided, create window invisible to prevent flickering
	if parentHWND != 0 && runtime.GOOS == "windows" {
		// Create window invisible - it will be shown after embedding
		glfw.WindowHint(glfw.Visible, glfw.False)
	}

	// Create window (invisible if parentHWND is provided)
	window, err := glfw.CreateWindow(previewWidth, previewHeight, windowTitle, nil, nil)
	if err != nil {
		log.Fatalln("Error creating preview window:", err)
	}

	// If parent HWND is provided, ensure window is hidden and embed it
	if parentHWND != 0 && runtime.GOOS == "windows" {
		// Double-check: hide window immediately via Win32 API (hint might not be enough)
		// This ensures window is hidden even if GLFW hint didn't work
		hideWindow(window, windowTitle)
		// Process events to ensure hide command is registered
		glfw.PollEvents()
		// Small delay to ensure window is fully hidden
		time.Sleep(5 * time.Millisecond)
		// Embed the window (it will be shown automatically after embedding)
		previewWidth, previewHeight = embedWindowIntoParent(window, parentHWND, windowTitle)
	}

	window.MakeContextCurrent()

	if err := gl.Init(); err != nil {
		log.Fatalln("Error initializing OpenGL:", err)
	}

	// Disable depth test for fullscreen quad
	gl.Disable(gl.DEPTH_TEST)

	// Create fullscreen quad
	quad := createFullscreenQuad()

	// Load shader from file
	var program uint32
	shaderData, err := loadEmbeddedShader()
	if err != nil {
		log.Fatalf("Error loading shader: %v", err)
	}

	vertexShader, fragmentShader, err := getMainShaderCode(shaderData)
	if err != nil {
		log.Fatalf("Error extracting shader code: %v", err)
	}

	// Debug: output shader information
	if DEBUG_MODE {
		log.Printf("Shader loaded successfully")
		log.Printf("Fragment shader length: %d bytes", len(fragmentShader))
		// Find mainImage in code
		if strings.Contains(fragmentShader, "mainImage") {
			log.Printf("mainImage function found in shader code")
		} else {
			log.Printf("WARNING: mainImage function NOT found in shader code!")
		}
	}

	program = newProgram(vertexShader, fragmentShader)

	// Get shader uniform variable locations
	iResolutionLoc := gl.GetUniformLocation(program, gl.Str("iResolution\x00"))
	iTimeLoc := gl.GetUniformLocation(program, gl.Str("iTime\x00"))
	iTimeDeltaLoc := gl.GetUniformLocation(program, gl.Str("iTimeDelta\x00"))
	iFrameLoc := gl.GetUniformLocation(program, gl.Str("iFrame\x00"))
	iFrameRateLoc := gl.GetUniformLocation(program, gl.Str("iFrameRate\x00"))
	iMouseLoc := gl.GetUniformLocation(program, gl.Str("iMouse\x00"))
	iDateLoc := gl.GetUniformLocation(program, gl.Str("iDate\x00"))
	iSampleRateLoc := gl.GetUniformLocation(program, gl.Str("iSampleRate\x00"))
	iChannelResolutionLoc := gl.GetUniformLocation(program, gl.Str("iChannelResolution\x00"))
	iChannelTimeLoc := gl.GetUniformLocation(program, gl.Str("iChannelTime\x00"))
	iFadeLoc := gl.GetUniformLocation(program, gl.Str("iFade\x00"))

	// Debug: check for main uniforms
	if DEBUG_MODE {
		log.Printf("Uniform locations: iResolution=%d, iTime=%d, iTimeDelta=%d, iFrame=%d",
			iResolutionLoc, iTimeLoc, iTimeDeltaLoc, iFrameLoc)
		if iResolutionLoc < 0 {
			log.Println("WARNING: iResolution uniform not found in shader!")
		}
		if iTimeLoc < 0 {
			log.Println("WARNING: iTime uniform not found in shader!")
		}
	}

	// Flag to signal graceful exit (show black screen before closing)
	shouldExit := false
	var exitStartTime time.Time

	startTime := time.Now()
	lastTime := startTime
	frameCount := 0

	for !window.ShouldClose() {
		currentTime := time.Now()
		elapsed := currentTime.Sub(startTime).Seconds()
		deltaTime := currentTime.Sub(lastTime).Seconds()
		lastTime = currentTime
		frameCount++

		// Calculate fade value: fade-in over 1 second, fade-out over 0.5 seconds
		var fadeValue float32 = 1.0
		if elapsed < 1.0 {
			// Fade-in: 0 to 1 over 1 second
			fadeValue = float32(elapsed)
		} else if shouldExit {
			// Fade-out: 1 to 0 over 0.5 seconds
			if exitStartTime.IsZero() {
				exitStartTime = currentTime
			}
			exitElapsed := currentTime.Sub(exitStartTime).Seconds()
			if exitElapsed < 0.5 {
				fadeValue = float32(1.0 - exitElapsed/0.5)
			} else {
				fadeValue = 0.0
			}
		}

		// Use framebuffer size instead of window size for correct viewport
		fbWidth, fbHeight := window.GetFramebufferSize()
		width, height := window.GetSize()

		// Set viewport based on framebuffer size
		gl.Viewport(0, 0, int32(fbWidth), int32(fbHeight))

		gl.ClearColor(0.0, 0.0, 0.0, 1.0)
		gl.Clear(gl.COLOR_BUFFER_BIT)

		gl.UseProgram(program)

		// Set shader uniforms
		if iResolutionLoc >= 0 {
			// iResolution: .xy = viewport size, .z = aspect ratio (width/height)
			// Use framebuffer size for correct resolution
			aspectRatio := float32(fbWidth) / float32(fbHeight)
			gl.Uniform3f(iResolutionLoc, float32(fbWidth), float32(fbHeight), aspectRatio)
			if DEBUG_MODE && frameCount == 1 {
				log.Printf("Setting iResolution to: %.0f x %.0f (aspect: %.3f)", float32(width), float32(height), aspectRatio)
			}
		}
		if iTimeLoc >= 0 {
			gl.Uniform1f(iTimeLoc, float32(elapsed))
			if DEBUG_MODE && frameCount == 1 {
				log.Printf("Setting iTime to: %.2f", float32(elapsed))
			}
		}
		if iTimeDeltaLoc >= 0 {
			gl.Uniform1f(iTimeDeltaLoc, float32(deltaTime))
		}
		if iFrameLoc >= 0 {
			gl.Uniform1i(iFrameLoc, int32(frameCount))
		}
		if iFrameRateLoc >= 0 {
			// Calculate FPS for iFrameRate
			currentFPS := float32(1.0 / deltaTime)
			if deltaTime <= 0 {
				currentFPS = 60.0 // fallback
			}
			gl.Uniform1f(iFrameRateLoc, currentFPS)
		}
		// Mock mouse (no input in screensaver)
		// iMouse.xy = current position, iMouse.zw = click position (should be < 0 if not pressed)
		if iMouseLoc >= 0 {
			gl.Uniform4f(iMouseLoc, 0.0, 0.0, -1.0, -1.0) // x, y, click x, click y (not pressed)
		}
		// Mock date
		if iDateLoc >= 0 {
			now := time.Now()
			gl.Uniform4f(iDateLoc, float32(now.Year()), float32(now.Month()), float32(now.Day()), float32(elapsed))
		}
		if iSampleRateLoc >= 0 {
			gl.Uniform1f(iSampleRateLoc, 44100.0) // Standard sample rate
		}
		// Mock channel resolution and time
		if iChannelResolutionLoc >= 0 {
			resolutions := []float32{float32(width), float32(height), 0.0, float32(width), float32(height), 0.0, float32(width), float32(height), 0.0, float32(width), float32(height), 0.0}
			gl.Uniform3fv(iChannelResolutionLoc, 4, &resolutions[0])
		}
		if iChannelTimeLoc >= 0 {
			times := []float32{float32(elapsed), float32(elapsed), float32(elapsed), float32(elapsed)}
			gl.Uniform1fv(iChannelTimeLoc, 4, &times[0])
		}
		// Set fade uniform for smooth fade-in/fade-out
		if iFadeLoc >= 0 {
			gl.Uniform1f(iFadeLoc, fadeValue)
		}

		// Draw fullscreen quad
		gl.BindVertexArray(quad.vao)
		gl.DrawElements(gl.TRIANGLES, 6, gl.UNSIGNED_INT, gl.PtrOffset(0))

		window.SwapBuffers()
		glfw.PollEvents()

		// Exit loop if fade-out is complete
		if shouldExit && !exitStartTime.IsZero() {
			exitElapsed := currentTime.Sub(exitStartTime).Seconds()
			if exitElapsed >= 0.5 {
				// Fade-out complete, exit loop
				break
			}
		}
	}

	// Graceful exit: show black screen before closing
	if shouldExit {
		// Get framebuffer size for viewport
		fbWidth, fbHeight := window.GetFramebufferSize()

		// Clear to black
		gl.Viewport(0, 0, int32(fbWidth), int32(fbHeight))
		gl.ClearColor(0.0, 0.0, 0.0, 1.0)
		gl.Clear(gl.COLOR_BUFFER_BIT)
		window.SwapBuffers()

		// Wait a bit to ensure black screen is displayed
		time.Sleep(100 * time.Millisecond)

		// Process events to ensure black screen is shown
		glfw.PollEvents()

		// Now close the window
		window.SetShouldClose(true)
		glfw.PollEvents()
	}
}

// FullscreenQuad structure for fullscreen quad
type FullscreenQuad struct {
	vao uint32
	vbo uint32
}

// ShaderInput represents one input channel/texture in shader JSON.
type ShaderInput struct {
	ID      string `json:"id"`
	Channel int    `json:"channel"`
	Src     string `json:"src,omitempty"`
	Type    string `json:"type,omitempty"`
}

// ShaderPass represents one shader pass.
type ShaderPass struct {
	Index  int              `json:"index,omitempty"`
	Code   string           `json:"code"`
	Inputs []ShaderInput `json:"inputs,omitempty"`
	Type   string           `json:"type,omitempty"`
	Name   string           `json:"name,omitempty"`
}

// ShaderData represents shader JSON file structure.
type ShaderData struct {
	Metadata      *ShaderMetadata       `json:"metadata,omitempty"`
	Passes        []ShaderPass          `json:"passes"`
	Screenshots   []string              `json:"screenshots,omitempty"`
	Performance   *ShaderPerformance    `json:"performance,omitempty"`
	InputTextures []interface{}         `json:"input_textures,omitempty"`
	PassTextures  []interface{}         `json:"pass_textures,omitempty"`
}

// ShaderMetadata represents metadata in shader JSON.
type ShaderMetadata struct {
	URL         string `json:"url,omitempty"`
	ShaderID    string `json:"shader_id,omitempty"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	NumPasses   int    `json:"num_passes,omitempty"`
}

// ShaderPerformance represents performance metrics in shader JSON.
type ShaderPerformance struct {
	CPUUsagePercent float64 `json:"cpu_usage_percent,omitempty"`
	GPUUsagePercent float64 `json:"gpu_usage_percent,omitempty"`
}

const textVertexShaderSource = `
#version 330 core
layout(location = 0) in vec2 aPos;
layout(location = 1) in vec2 aTexCoord;
out vec2 TexCoord;
uniform mat4 projection;

void main() {
    // Use z = -0.99 so text is as close to camera as possible
    gl_Position = projection * vec4(aPos, -0.99, 1.0);
    TexCoord = aTexCoord;
}` + "\x00"

const textFragmentShaderSource = `
#version 330 core
in vec2 TexCoord;
out vec4 FragColor;
uniform sampler2D textTexture;
uniform vec3 textColor;

void main() {
    vec4 sampled = vec4(1.0, 1.0, 1.0, texture(textTexture, TexCoord).r);
    FragColor = vec4(textColor, 1.0) * sampled;
}` + "\x00"

func compileShader(source string, shaderType uint32) uint32 {
	shader := gl.CreateShader(shaderType)
	csources, free := gl.Strs(source)
	gl.ShaderSource(shader, 1, csources, nil)
	free()
	gl.CompileShader(shader)

	var status int32
	gl.GetShaderiv(shader, gl.COMPILE_STATUS, &status)
	if status == gl.FALSE {
		var logLength int32
		gl.GetShaderiv(shader, gl.INFO_LOG_LENGTH, &logLength)
		logBytes := make([]byte, logLength)
		gl.GetShaderInfoLog(shader, logLength, nil, &logBytes[0])
		shaderTypeStr := "vertex"
		if shaderType == gl.FRAGMENT_SHADER {
			shaderTypeStr = "fragment"
		}
		errorLog := string(logBytes)
		log.Printf("Error compiling %s shader:\n%s", shaderTypeStr, errorLog)
		if DEBUG_MODE {
			// Output full shader source code for debugging
			log.Printf("Full shader source code:\n%s", source)
			// Try to extract line number from error message
			if strings.Contains(errorLog, ":") {
				// Error messages often contain line numbers like "ERROR: 0:123: ..."
				log.Printf("Check the line number in the error message above")
			}
		}
		log.Fatalln("Failed to compile shader")
	}
	return shader
}

// createFullscreenQuad creates fullscreen quad for fragment shader rendering.
func createFullscreenQuad() *FullscreenQuad {
	// Fullscreen quad vertices (0.0 to 1.0 for texture coordinates)
	vertices := []float32{
		// x, y, u, v
		0.0, 0.0, 0.0, 0.0, // bottom left
		1.0, 0.0, 1.0, 0.0, // bottom right
		1.0, 1.0, 1.0, 1.0, // top right
		0.0, 1.0, 0.0, 1.0, // top left
	}

	indices := []uint32{
		0, 1, 2,
		0, 2, 3,
	}

	var vao, vbo, ebo uint32
	gl.GenVertexArrays(1, &vao)
	gl.GenBuffers(1, &vbo)
	gl.GenBuffers(1, &ebo)

	gl.BindVertexArray(vao)

	gl.BindBuffer(gl.ARRAY_BUFFER, vbo)
	gl.BufferData(gl.ARRAY_BUFFER, len(vertices)*4, gl.Ptr(vertices), gl.STATIC_DRAW)

	gl.BindBuffer(gl.ELEMENT_ARRAY_BUFFER, ebo)
	gl.BufferData(gl.ELEMENT_ARRAY_BUFFER, len(indices)*4, gl.Ptr(indices), gl.STATIC_DRAW)

	// Position (location 0)
	gl.VertexAttribPointer(0, 2, gl.FLOAT, false, 4*4, gl.PtrOffset(0))
	gl.EnableVertexAttribArray(0)

	// Texture coordinates (location 1) - used as normalized fragment coordinates.
	gl.VertexAttribPointer(1, 2, gl.FLOAT, false, 4*4, gl.PtrOffset(2*4))
	gl.EnableVertexAttribArray(1)

	gl.BindVertexArray(0)

	return &FullscreenQuad{
		vao: vao,
		vbo: vbo,
	}
}

func newProgram(vertexSrc, fragmentSrc string) uint32 {
	vertexShader := compileShader(vertexSrc, gl.VERTEX_SHADER)
	fragmentShader := compileShader(fragmentSrc, gl.FRAGMENT_SHADER)

	program := gl.CreateProgram()
	gl.AttachShader(program, vertexShader)
	gl.AttachShader(program, fragmentShader)
	gl.LinkProgram(program)

	var status int32
	gl.GetProgramiv(program, gl.LINK_STATUS, &status)
	if status == gl.FALSE {
		var logLength int32
		gl.GetProgramiv(program, gl.INFO_LOG_LENGTH, &logLength)
		logBytes := make([]byte, logLength)
		gl.GetProgramInfoLog(program, logLength, nil, &logBytes[0])
		log.Fatalln("Error linking shader program:", string(logBytes))
	}

	gl.DeleteShader(vertexShader)
	gl.DeleteShader(fragmentShader)
	return program
}

type TextRenderer struct {
	program    uint32
	vao        uint32
	vbo        uint32
	texture    uint32
	projection int32
	textColor  int32
	width      int
	height     int
}

func newTextRenderer(window *glfw.Window) *TextRenderer {
	tr := &TextRenderer{}

	// Create shader program for text
	tr.program = newProgram(textVertexShaderSource, textFragmentShaderSource)
	tr.projection = gl.GetUniformLocation(tr.program, gl.Str("projection\x00"))
	tr.textColor = gl.GetUniformLocation(tr.program, gl.Str("textColor\x00"))

	// Create VAO and VBO for quad (text texture)
	var vao, vbo uint32
	gl.GenVertexArrays(1, &vao)
	gl.GenBuffers(1, &vbo)

	gl.BindVertexArray(vao)
	gl.BindBuffer(gl.ARRAY_BUFFER, vbo)
	gl.BufferData(gl.ARRAY_BUFFER, 6*4*4, nil, gl.DYNAMIC_DRAW)

	gl.EnableVertexAttribArray(0)
	gl.VertexAttribPointer(0, 2, gl.FLOAT, false, 4*4, gl.PtrOffset(0))
	gl.EnableVertexAttribArray(1)
	gl.VertexAttribPointer(1, 2, gl.FLOAT, false, 4*4, gl.PtrOffset(2*4))

	gl.BindBuffer(gl.ARRAY_BUFFER, 0)
	gl.BindVertexArray(0)

	tr.vao = vao
	tr.vbo = vbo

	// Create texture for text
	var texture uint32
	gl.GenTextures(1, &texture)
	gl.BindTexture(gl.TEXTURE_2D, texture)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.LINEAR)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.LINEAR)
	gl.BindTexture(gl.TEXTURE_2D, 0)

	tr.texture = texture

	width, height := window.GetSize()
	tr.width = width
	tr.height = height

	return tr
}

func (tr *TextRenderer) Render(text string, x, y float32, scale float32) {
	// Disable depth testing for text so it's always visible on top
	gl.Disable(gl.DEPTH_TEST)
	gl.Enable(gl.BLEND)
	gl.BlendFunc(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA)

	// Create image with text
	img := image.NewRGBA(image.Rect(0, 0, 512, 64))
	draw.Draw(img, img.Bounds(), image.NewUniform(color.RGBA{0, 0, 0, 0}), image.Point{}, draw.Src)

	// Draw text
	// Y position: basicfont.Face7x13 has Ascent of about 13 pixels
	// Use 13 * 64 (fixed point) so text is in upper part of image
	d := &font.Drawer{
		Dst:  img,
		Src:  image.NewUniform(color.RGBA{255, 255, 255, 255}),
		Face: basicfont.Face7x13,
		Dot:  fixed.Point26_6{X: fixed.Int26_6(0), Y: fixed.Int26_6(13 * 64)},
	}
	d.DrawString(text)

	// Load texture
	gl.BindTexture(gl.TEXTURE_2D, tr.texture)
	gl.TexImage2D(gl.TEXTURE_2D, 0, gl.RED, int32(img.Bounds().Dx()), int32(img.Bounds().Dy()), 0, gl.RGBA, gl.UNSIGNED_BYTE, gl.Ptr(img.Pix))

	w := float32(img.Bounds().Dx()) * scale
	h := float32(img.Bounds().Dy()) * scale

	// Set orthographic projection
	// Invert Y so (0,0) is at top-left corner
	projection := []float32{
		2.0 / float32(tr.width), 0, 0, 0,
		0, -2.0 / float32(tr.height), 0, 0, // minus for Y inversion
		0, 0, -1, 0,
		-1, 1, 0, 1, // offset: -1 on X, 1 on Y (instead of -1, -1)
	}

	gl.UseProgram(tr.program)
	gl.UniformMatrix4fv(tr.projection, 1, false, &projection[0])
	gl.Uniform3f(tr.textColor, 1.0, 1.0, 1.0) // White text color

	gl.BindVertexArray(tr.vao)

	// Create quad for text
	// Invert texture coordinates on Y since Y is inverted in projection
	vertices := []float32{
		x, y + h, 0.0, 1.0, // bottom left vertex -> bottom left texture
		x, y, 0.0, 0.0, // top left vertex -> top left texture
		x + w, y, 1.0, 0.0, // top right vertex -> top right texture
		x, y + h, 0.0, 1.0, // bottom left vertex -> bottom left texture
		x + w, y, 1.0, 0.0, // top right vertex -> top right texture
		x + w, y + h, 1.0, 1.0, // bottom right vertex -> bottom right texture
	}

	gl.BindBuffer(gl.ARRAY_BUFFER, tr.vbo)
	gl.BufferSubData(gl.ARRAY_BUFFER, 0, len(vertices)*4, gl.Ptr(vertices))

	gl.ActiveTexture(gl.TEXTURE0)
	gl.BindTexture(gl.TEXTURE_2D, tr.texture)

	gl.DrawArrays(gl.TRIANGLES, 0, 6)

	gl.BindVertexArray(0)
	gl.BindTexture(gl.TEXTURE_2D, 0)
	gl.Disable(gl.BLEND)
	// Do NOT enable depth test back - for fullscreen quad it should be disabled
	// gl.Enable(gl.DEPTH_TEST) - removed, as main shader doesn't use depth test
}

// runScreensaverMode starts fullscreen screensaver
func runScreensaverMode() {
	if err := glfw.Init(); err != nil {
		log.Fatalln("Error initializing GLFW:", err)
	}
	defer glfw.Terminate()

	glfw.WindowHint(glfw.Resizable, glfw.False)
	glfw.WindowHint(glfw.ContextVersionMajor, 3)
	glfw.WindowHint(glfw.ContextVersionMinor, 3)
	glfw.WindowHint(glfw.OpenGLProfile, glfw.OpenGLCoreProfile)
	glfw.WindowHint(glfw.OpenGLForwardCompatible, glfw.True)
	glfw.WindowHint(glfw.Samples, 4) // Enable multisampling with 4 samples for antialiasing

	var window *glfw.Window
	var err error

	// Build window title with command line arguments in debug mode
	windowTitle := SCREENSAVER_NAME
	if DEBUG_MODE {
		// Only show command line arguments (skip program name/path)
		if len(os.Args) > 1 {
			argsStr := strings.Join(os.Args[1:], " ")
			windowTitle = fmt.Sprintf("[Args: %s]", argsStr)
		} else {
			windowTitle = "[Args: (none)]"
		}
	}

	if FULLSCREEN_MODE {
		// Get primary monitor for fullscreen mode
		monitor := glfw.GetPrimaryMonitor()
		mode := monitor.GetVideoMode()
		window, err = glfw.CreateWindow(mode.Width, mode.Height, windowTitle, monitor, nil)
	} else {
		// Windowed mode
		window, err = glfw.CreateWindow(800, 600, windowTitle, nil, nil)
	}

	if err != nil {
		log.Fatalln("Error creating window:", err)
	}
	window.MakeContextCurrent()

	// Flag to signal graceful exit (show black screen before closing)
	shouldExit := false
	var exitStartTime time.Time

	// Set handlers to exit program on any key or mouse button press
	if EXIT_ON_KEY_PRESS {
		window.SetKeyCallback(func(w *glfw.Window, key glfw.Key, scancode int, action glfw.Action, mods glfw.ModifierKey) {
			if action == glfw.Press {
				shouldExit = true
				if exitStartTime.IsZero() {
					exitStartTime = time.Now()
				}
			}
		})
	}

	if EXIT_ON_MOUSE_CLICK {
		window.SetMouseButtonCallback(func(w *glfw.Window, button glfw.MouseButton, action glfw.Action, mods glfw.ModifierKey) {
			if action == glfw.Press {
				shouldExit = true
				if exitStartTime.IsZero() {
					exitStartTime = time.Now()
				}
			}
		})
	}

	// Hide mouse cursor if needed
	if HIDE_MOUSE_CURSOR {
		window.SetInputMode(glfw.CursorMode, glfw.CursorHidden)
	}

	if err := gl.Init(); err != nil {
		log.Fatalln("Error initializing OpenGL:", err)
	}

	// Enable multisampling for antialiasing
	gl.Enable(gl.MULTISAMPLE)

	// Disable depth test for fullscreen quad
	gl.Disable(gl.DEPTH_TEST)

	// Create fullscreen quad
	quad := createFullscreenQuad()

	// Load shader from file
	var program uint32
	shaderData, err := loadEmbeddedShader()
	if err != nil {
		log.Fatalf("Error loading shader: %v", err)
	}

	vertexShader, fragmentShader, err := getMainShaderCode(shaderData)
	if err != nil {
		log.Fatalf("Error extracting shader code: %v", err)
	}

	// Debug: output shader information
	if DEBUG_MODE {
		log.Printf("Shader loaded successfully")
		log.Printf("Fragment shader length: %d bytes", len(fragmentShader))
		// Find mainImage in code
		if strings.Contains(fragmentShader, "mainImage") {
			log.Printf("mainImage function found in shader code")
		} else {
			log.Printf("WARNING: mainImage function NOT found in shader code!")
		}
	}

	program = newProgram(vertexShader, fragmentShader)

	// Get shader uniform variable locations
	iResolutionLoc := gl.GetUniformLocation(program, gl.Str("iResolution\x00"))
	iTimeLoc := gl.GetUniformLocation(program, gl.Str("iTime\x00"))
	iTimeDeltaLoc := gl.GetUniformLocation(program, gl.Str("iTimeDelta\x00"))
	iFrameLoc := gl.GetUniformLocation(program, gl.Str("iFrame\x00"))
	iFrameRateLoc := gl.GetUniformLocation(program, gl.Str("iFrameRate\x00"))
	iMouseLoc := gl.GetUniformLocation(program, gl.Str("iMouse\x00"))
	iDateLoc := gl.GetUniformLocation(program, gl.Str("iDate\x00"))
	iSampleRateLoc := gl.GetUniformLocation(program, gl.Str("iSampleRate\x00"))
	iChannelResolutionLoc := gl.GetUniformLocation(program, gl.Str("iChannelResolution\x00"))
	iChannelTimeLoc := gl.GetUniformLocation(program, gl.Str("iChannelTime\x00"))
	iFadeLoc := gl.GetUniformLocation(program, gl.Str("iFade\x00"))

	// Debug: check for main uniforms
	if DEBUG_MODE {
		log.Printf("Uniform locations: iResolution=%d, iTime=%d, iTimeDelta=%d, iFrame=%d",
			iResolutionLoc, iTimeLoc, iTimeDeltaLoc, iFrameLoc)
		if iResolutionLoc < 0 {
			log.Println("WARNING: iResolution uniform not found in shader!")
		}
		if iTimeLoc < 0 {
			log.Println("WARNING: iTime uniform not found in shader!")
		}
	}

	// Create text renderer
	textRenderer := newTextRenderer(window)

	// Variables for FPS
	startTime := time.Now()
	lastTime := time.Now()
	frameCount := 0
	fpsUpdateTime := lastTime
	fps := 0.0

	// Variables for average frame time over last 5 seconds
	type frameTimeEntry struct {
		time  time.Time
		delta float64
	}
	frameTimes := make([]frameTimeEntry, 0)
	const frameTimeWindow = 5 * time.Second

	for !window.ShouldClose() {
		currentTime := time.Now()
		deltaTime := currentTime.Sub(lastTime).Seconds()
		lastTime = currentTime

		// Update FPS every second
		frameCount++
		if currentTime.Sub(fpsUpdateTime) >= time.Second {
			fps = float64(frameCount) / currentTime.Sub(fpsUpdateTime).Seconds()
			frameCount = 0
			fpsUpdateTime = currentTime
		}

		elapsed := currentTime.Sub(startTime).Seconds()

		// Calculate fade value: fade-in over 1 second, fade-out over 0.5 seconds
		var fadeValue float32 = 1.0
		if elapsed < 1.0 {
			// Fade-in: 0 to 1 over 1 second
			fadeValue = float32(elapsed)
		} else if shouldExit {
			// Fade-out: 1 to 0 over 0.5 seconds
			if exitStartTime.IsZero() {
				exitStartTime = currentTime
			}
			exitElapsed := currentTime.Sub(exitStartTime).Seconds()
			if exitElapsed < 0.5 {
				fadeValue = float32(1.0 - exitElapsed/0.5)
			} else {
				fadeValue = 0.0
			}
		}
		// Use framebuffer size instead of window size for correct viewport
		fbWidth, fbHeight := window.GetFramebufferSize()
		width, height := window.GetSize()

		// Set viewport based on framebuffer size
		gl.Viewport(0, 0, int32(fbWidth), int32(fbHeight))

		gl.ClearColor(0.0, 0.0, 0.0, 1.0)
		gl.Clear(gl.COLOR_BUFFER_BIT)

		// Start render time measurement (shader execution time)
		renderStartTime := time.Now()

		gl.UseProgram(program)

		// Set shader uniforms
		if iResolutionLoc >= 0 {
			// iResolution: .xy = viewport size, .z = aspect ratio (width/height)
			// Use framebuffer size for correct resolution
			aspectRatio := float32(fbWidth) / float32(fbHeight)
			gl.Uniform3f(iResolutionLoc, float32(fbWidth), float32(fbHeight), aspectRatio)
			if DEBUG_MODE && frameCount == 1 {
				log.Printf("Setting iResolution to: %.0f x %.0f (aspect: %.3f)", float32(width), float32(height), aspectRatio)
			}
		}
		if iTimeLoc >= 0 {
			gl.Uniform1f(iTimeLoc, float32(elapsed))
			if DEBUG_MODE && frameCount == 1 {
				log.Printf("Setting iTime to: %.2f", float32(elapsed))
			}
		}
		if iTimeDeltaLoc >= 0 {
			gl.Uniform1f(iTimeDeltaLoc, float32(deltaTime))
		}
		if iFrameLoc >= 0 {
			gl.Uniform1i(iFrameLoc, int32(frameCount))
		}
		if iFrameRateLoc >= 0 {
			// Calculate FPS for iFrameRate
			currentFPS := float32(1.0 / deltaTime)
			if deltaTime <= 0 {
				currentFPS = 60.0 // fallback
			}
			gl.Uniform1f(iFrameRateLoc, currentFPS)
		}
		// Mock mouse (no input in screensaver)
		// iMouse.xy = current position, iMouse.zw = click position (should be < 0 if not pressed)
		if iMouseLoc >= 0 {
			gl.Uniform4f(iMouseLoc, 0.0, 0.0, -1.0, -1.0) // x, y, click x, click y (not pressed)
		}
		// Mock date
		if iDateLoc >= 0 {
			now := time.Now()
			gl.Uniform4f(iDateLoc, float32(now.Year()), float32(now.Month()), float32(now.Day()), float32(elapsed))
		}
		if iSampleRateLoc >= 0 {
			gl.Uniform1f(iSampleRateLoc, 44100.0) // Standard sample rate
		}
		// Mock channel resolution and time
		if iChannelResolutionLoc >= 0 {
			resolutions := []float32{float32(fbWidth), float32(fbHeight), 0.0, float32(fbWidth), float32(fbHeight), 0.0, float32(fbWidth), float32(fbHeight), 0.0, float32(fbWidth), float32(fbHeight), 0.0}
			gl.Uniform3fv(iChannelResolutionLoc, 4, &resolutions[0])
		}
		if iChannelTimeLoc >= 0 {
			times := []float32{float32(elapsed), float32(elapsed), float32(elapsed), float32(elapsed)}
			gl.Uniform1fv(iChannelTimeLoc, 4, &times[0])
		}
		// Set fade uniform for smooth fade-in/fade-out
		if iFadeLoc >= 0 {
			gl.Uniform1f(iFadeLoc, fadeValue)
		}

		// Draw fullscreen quad
		// Make sure program is still active before drawing
		gl.UseProgram(program)
		gl.BindVertexArray(quad.vao)
		gl.DrawElements(gl.TRIANGLES, 6, gl.UNSIGNED_INT, gl.PtrOffset(0))

		// Wait for all GPU commands to complete for accurate render time measurement
		gl.Finish()

		// Finish render time measurement
		renderEndTime := time.Now()
		renderTime := renderEndTime.Sub(renderStartTime).Seconds()

		// Add render time to history
		frameTimes = append(frameTimes, frameTimeEntry{
			time:  currentTime,
			delta: renderTime,
		})

		// Remove entries older than 5 seconds
		cutoffTime := currentTime.Add(-frameTimeWindow)
		validStart := 0
		for i, entry := range frameTimes {
			if entry.time.After(cutoffTime) {
				validStart = i
				break
			}
		}
		if validStart > 0 {
			frameTimes = frameTimes[validStart:]
		}

		// Display debug information if debug mode is enabled
		if DEBUG_MODE {
			// Calculate average frame time over last 5 seconds
			avgFrameTime := 0.0
			if len(frameTimes) > 0 {
				sum := 0.0
				for _, entry := range frameTimes {
					sum += entry.delta
				}
				avgFrameTime = sum / float64(len(frameTimes)) * 1000.0 // in milliseconds
			}
			// Update size in TextRenderer for correct projection (use framebuffer size for projection)
			textRenderer.width = fbWidth
			textRenderer.height = fbHeight
			// Render text (coordinates: x, y from top-left corner)
			// Display window size, not framebuffer (window size is more important for user)
			textRenderer.Render(fmt.Sprintf("Window: %dx%d, Framebuffer: %dx%d", width, height, fbWidth, fbHeight), 10, 2, 1.0)
			textRenderer.Render(fmt.Sprintf("FPS: %.1f", fps), 10, 15, 1.0)
			textRenderer.Render(fmt.Sprintf("Render Time: %.2f ms (avg 5s)", avgFrameTime), 10, 28, 1.0)
		}

		window.SwapBuffers()
		glfw.PollEvents()

		// Exit loop if fade-out is complete
		if shouldExit && !exitStartTime.IsZero() {
			exitElapsed := currentTime.Sub(exitStartTime).Seconds()
			if exitElapsed >= 0.5 {
				// Fade-out complete, exit loop
				break
			}
		}
	}

	// Graceful exit: window is already black after fade-out, just close
	if shouldExit {
		window.SetShouldClose(true)
		glfw.PollEvents()
	}
}

func main() {
	// If forced settings mode is enabled, start configuration dialog
	if FORCE_SETTINGS_MODE {
		runConfigMode()
		return
	}

	// Determine screensaver operation mode from command line arguments
	mode, parentHWND := detectScreensaverMode()

	switch mode {
	case ModeConfig:
		// Configuration mode - show dialog
		runConfigMode()
	case ModePreview:
		// Preview mode - small window
		runPreviewMode(parentHWND)
	case ModeScreensaver:
		fallthrough
	default:
		// Screensaver mode - fullscreen mode
		runScreensaverMode()
	}
}
