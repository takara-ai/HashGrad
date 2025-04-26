package main

import (
	"crypto/sha256"
	"encoding/binary"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"log"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"sync"
	"time"
)

const (
	imgWidth  = 800
	imgHeight = 600
)

// --- Color Palettes (Define globally) ---
var (
	defaultPalette = []string{"#d91009", "#FFFFFF", "#4A4D4E"} // Red, White, Gray
	bluePalette    = []string{"#001f3f", "#7FDBFF", "#FFFFFF"}    // Navy, Aqua, White
	earthPalette   = []string{"#8B4513", "#F4A460", "#2E8B57"}   // SaddleBrown, SandyBrown, SeaGreen
	japanesePalette = []string{"#BC002D", "#F3F3F2", "#2D2926"}  // Japanese Red, Off-White, Charcoal Gray
)

// hexToRGBA converts a hex color string to color.RGBA
func hexToRGBA(hex string) (color.RGBA, error) {
	var r, g, b uint8

	if len(hex) != 4 && len(hex) != 7 {
		return color.RGBA{}, fmt.Errorf("invalid hex color length: must be #RGB or #RRGGBB, got length %d", len(hex))
	}
	if hex[0] != '#' {
		return color.RGBA{}, fmt.Errorf("invalid hex color format: must start with #")
	}

	if len(hex) == 4 { // Handle short hex like #RGB
		format := "#%1x%1x%1x"
		_, err := fmt.Sscanf(hex, format, &r, &g, &b)
		if err != nil {
			return color.RGBA{}, fmt.Errorf("invalid short hex color format (%s): %w", hex, err)
		}
		r *= 17 // F becomes FF
		g *= 17
		b *= 17
	} else { // Must be len(hex) == 7
		format := "#%02x%02x%02x"
		_, err := fmt.Sscanf(hex, format, &r, &g, &b)
		if err != nil {
			return color.RGBA{}, fmt.Errorf("invalid hex color format (%s): %w", hex, err)
		}
	}
	return color.RGBA{R: r, G: g, B: b, A: 255}, nil
}

// Helper function to reverse a string
func reverseString(s string) string {
	runes := []rune(s)
	for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
		runes[i], runes[j] = runes[j], runes[i]
	}
	return string(runes)
}

func readUint32(b []byte) uint32 {
	return binary.BigEndian.Uint32(b)
}

func readUint16(b []byte) uint16 {
	return binary.BigEndian.Uint16(b)
}

// TestParameters holds the parameters we want to test
type TestParameters struct {
	angleSeed    float64
	warpFreqX    float64
	warpAmpX     float64 // Use 0 to disable warp X
	warpFreqY    float64
	warpAmpY     float64 // Use 0 to disable warp Y
	hillFreq     float64
	hillAmp      float64 // Use 0 to disable hill wave
	colorOrder   int
	description  string
	palette      *[]string
	inputStr     *string
	// Internal flag to indicate if it's an ablation test needing hash override
	// This is slightly hacky but avoids major refactoring of test setup
	isAblationOverride bool
}

// generateTestImage creates an image with specific parameters
func generateTestImage(params TestParameters) (*image.RGBA, map[string]float64, time.Duration) {
	input := "test"
	if params.inputStr != nil {
		input = *params.inputStr
	}
	hasher := sha256.New()
	hasher.Write([]byte(input))
	hashBytes := hasher.Sum(nil)

	// --- Override hash bytes based on TestParameters if needed ---
	// This allows forcing specific parameters *after* hashing the base string
	// Necessary for ablation tests where we want controlled parameters
	// while still starting from a consistent hash base.

	// Use a temporary buffer to modify bytes without affecting original hashBytes slice directly if not needed
	tempHashBytes := make([]byte, len(hashBytes))
	copy(tempHashBytes, hashBytes)
	// Check if any override is needed based on the parameter struct flag
	if params.isAblationOverride {
		// Angle Seed (Bytes 0-7)
		// Calculate the uint64 representation corresponding to the desired float64 seed [0, 1)
		angleSeedUint64 := uint64(params.angleSeed * float64(math.MaxUint64))
		binary.BigEndian.PutUint64(tempHashBytes[0:8], angleSeedUint64)

		// Warp X Freq (Bytes 8-11)
		// Note: Other overrides might have similar issues if the target value is float-based

		// Warp X Amp (Bytes 12-13) - Crucial for ablation
		binary.BigEndian.PutUint16(tempHashBytes[12:14], uint16(params.warpAmpX*float64(math.MaxUint16)))

		// Warp Y Freq (Bytes 16-19) - Note: Skipping bytes 14-15 (Warp X Phase) for simplicity here
		binary.BigEndian.PutUint32(tempHashBytes[16:20], uint32(params.warpFreqY*float64(math.MaxUint32)))
		// Warp Y Amp (Bytes 20-21) - Crucial for ablation
		binary.BigEndian.PutUint16(tempHashBytes[20:22], uint16(params.warpAmpY*float64(math.MaxUint16)))

		// Hill Freq (Bytes 24-27) - Note: Skipping bytes 22-23 (Warp Y Phase)
		binary.BigEndian.PutUint32(tempHashBytes[24:28], uint32(params.hillFreq*float64(math.MaxUint32)))
		// Hill Amp (Byte 31) - Note: Skipping bytes 28-30 (Hill Phase, Color Order)
		tempHashBytes[31] = byte(params.hillAmp * float64(math.MaxUint8))

		// Color Order (Byte 30)
		tempHashBytes[30] = byte(params.colorOrder)
	}

	// Use the potentially modified hash bytes
	finalHashBytes := tempHashBytes


	// --- Palette Setup ---
	paletteHex := defaultPalette
	if params.palette != nil {
		paletteHex = *params.palette
	}

	var currentPalette [3]color.RGBA
	var err error
	for i, hex := range paletteHex {
		if i >= 3 {
			break
		}
		currentPalette[i], err = hexToRGBA(hex)
		if err != nil {
			log.Printf("Warning: Invalid hex color '%s' in palette for test '%s'. Using white.", hex, params.description)
			currentPalette[i] = color.RGBA{R: 255, G: 255, B: 255, A: 255}
		}
	}

	// --- Generate Image ---
	start := time.Now()
	// Pass the potentially overridden hash bytes
	img, calculatedParams := generateGradientImage(finalHashBytes, currentPalette[0], currentPalette[1], currentPalette[2], imgWidth, imgHeight, false)
	duration := time.Since(start)

	// --- Record Input Parameters ---
	// Store the *intended* parameters used for generation in the map
	calculatedParams["_input_angleSeed"] = params.angleSeed
	calculatedParams["_input_warpFreqX"] = params.warpFreqX
	calculatedParams["_input_warpAmpX"] = params.warpAmpX
	calculatedParams["_input_warpFreqY"] = params.warpFreqY
	calculatedParams["_input_warpAmpY"] = params.warpAmpY
	calculatedParams["_input_hillFreq"] = params.hillFreq
	calculatedParams["_input_hillAmp"] = params.hillAmp
	calculatedParams["_input_colorOrder"] = float64(params.colorOrder)
	if params.inputStr != nil {
		calculatedParams["_input_string_used_"] = 1 // Indicate custom string used
	} else {
		calculatedParams["_input_string_used_"] = 0 // Indicate default "test" string used
	}
	if params.palette != nil {
		calculatedParams["_input_palette_used_"] = 1 // Indicate custom palette used
	} else {
		calculatedParams["_input_palette_used_"] = 0 // Indicate default palette used
	}


	return img, calculatedParams, duration
}

// saveTestImage saves an image with a descriptive filename and parameters
func saveTestImage(img image.Image, testType, filename, description string, calculatedParams map[string]float64, duration time.Duration) error {
	dir := filepath.Join("tests", testType)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	imgPath := filepath.Join(dir, filename)
	outFile, err := os.Create(imgPath)
	if err != nil {
		return fmt.Errorf("failed to create image file %s: %w", imgPath, err)
	}
	if err := png.Encode(outFile, img); err != nil {
		outFile.Close()
		return fmt.Errorf("failed to encode PNG %s: %w", imgPath, err)
	}
	if err := outFile.Close(); err != nil {
	    return fmt.Errorf("failed to close image file %s: %w", imgPath, err)
	}


	// Save description and parameters in a text file
	descPath := filepath.Join(dir, filename+".txt")
	descFile, err := os.Create(descPath)
	if err != nil {
		return fmt.Errorf("failed to create description file %s: %w", descPath, err)
	}
	defer descFile.Close()

	// Write description
	_, err = fmt.Fprintf(descFile, "Test Description: %s\n", description)
    if err != nil { return fmt.Errorf("write error: %w", err) }
    // Write Duration
    _, err = fmt.Fprintf(descFile, "Generation Duration: %s\n\n", duration.String())
    if err != nil { return fmt.Errorf("write error: %w", err) }


	// Write Input Seeds (from TestParameters struct)
	_, err = fmt.Fprintf(descFile, "--- Input Seeds ---\n")
    if err != nil { return fmt.Errorf("write error: %w", err) }
    // Extract input seeds from the calculatedParams map where they were stored
    _, err = fmt.Fprintf(descFile, "angleSeed: %.6f\n", calculatedParams["_input_angleSeed"])
    if err != nil { return fmt.Errorf("write error: %w", err) }
    _, err = fmt.Fprintf(descFile, "warpFreqX: %.6f\n", calculatedParams["_input_warpFreqX"])
    if err != nil { return fmt.Errorf("write error: %w", err) }
    _, err = fmt.Fprintf(descFile, "warpAmpX:  %.6f\n", calculatedParams["_input_warpAmpX"])
    if err != nil { return fmt.Errorf("write error: %w", err) }
    _, err = fmt.Fprintf(descFile, "warpFreqY: %.6f\n", calculatedParams["_input_warpFreqY"])
    if err != nil { return fmt.Errorf("write error: %w", err) }
    _, err = fmt.Fprintf(descFile, "warpAmpY:  %.6f\n", calculatedParams["_input_warpAmpY"])
    if err != nil { return fmt.Errorf("write error: %w", err) }
    _, err = fmt.Fprintf(descFile, "hillFreq:  %.6f\n", calculatedParams["_input_hillFreq"])
    if err != nil { return fmt.Errorf("write error: %w", err) }
    _, err = fmt.Fprintf(descFile, "hillAmp:   %.6f\n", calculatedParams["_input_hillAmp"])
    if err != nil { return fmt.Errorf("write error: %w", err) }
    _, err = fmt.Fprintf(descFile, "colorOrder: %.0f\n\n", calculatedParams["_input_colorOrder"])
    if err != nil { return fmt.Errorf("write error: %w", err) }


	// Write Calculated Parameters
	_, err = fmt.Fprintf(descFile, "--- Calculated Parameters ---\n")
    if err != nil { return fmt.Errorf("write error: %w", err) }
    // Sort keys for consistent output order
    keys := make([]string, 0, len(calculatedParams))
    for k := range calculatedParams {
        // Exclude the keys used to store input seeds
        if k[0] != '_' {
            keys = append(keys, k)
        }
    }
    sort.Strings(keys)

    for _, k := range keys {
        _, err = fmt.Fprintf(descFile, "%s: %.6f\n", k, calculatedParams[k])
        if err != nil {
            return fmt.Errorf("failed to write calculated param %s to %s: %w", k, descPath, err)
        }
    }

	return nil
}

// runParameterTests executes a series of parameter tests
func runParameterTests() {
	// Define strings to test sensitivity
	strShort := "hi"
	strLong := "a_very_long_test_string_with_symbols_!@#$%^&*()"
	strSamePrefix := "testing1"
	strSamePrefixDiff := "testing2"

	// Base parameters for consistency in ablation/comparison where applicable
	baseAngle := 0.25 // 90 deg
	baseWarpFreq := 0.5
	baseWarpAmp := 0.5
	baseHillFreq := 0.5
	baseHillAmp := 0.5
	baseColorOrder := 0


	// Test cases including new string and palette tests
	testCases := []struct {
		testType string
		params   []TestParameters
	}{
		{"angle", []TestParameters{
			// Use isAblationOverride: true to force angleSeed via hash override
			{0.0, baseWarpFreq, baseWarpAmp, baseWarpFreq, baseWarpAmp, baseHillFreq, baseHillAmp, baseColorOrder, "Angle: 0 deg", nil, nil, true},
			{0.25, baseWarpFreq, baseWarpAmp, baseWarpFreq, baseWarpAmp, baseHillFreq, baseHillAmp, baseColorOrder, "Angle: 90 deg", nil, nil, true},
			{0.5, baseWarpFreq, baseWarpAmp, baseWarpFreq, baseWarpAmp, baseHillFreq, baseHillAmp, baseColorOrder, "Angle: 180 deg", nil, nil, true},
			{0.75, baseWarpFreq, baseWarpAmp, baseWarpFreq, baseWarpAmp, baseHillFreq, baseHillAmp, baseColorOrder, "Angle: 270 deg", nil, nil, true},
		}},
		{"warp", []TestParameters{
			// Use isAblationOverride: true to force warpFreqX/Y via hash override
			{baseAngle, 0.1, baseWarpAmp, 0.1, baseWarpAmp, baseHillFreq, baseHillAmp, baseColorOrder, "Warp Freq: Low (0.1)", nil, nil, true},
			{baseAngle, 0.5, baseWarpAmp, 0.5, baseWarpAmp, baseHillFreq, baseHillAmp, baseColorOrder, "Warp Freq: Medium (0.5)", nil, nil, true},
			{baseAngle, 0.9, baseWarpAmp, 0.9, baseWarpAmp, baseHillFreq, baseHillAmp, baseColorOrder, "Warp Freq: High (0.9)", nil, nil, true},
			// Also test Warp Amplitude variation
			{baseAngle, baseWarpFreq, 0.1, baseWarpFreq, 0.1, baseHillFreq, baseHillAmp, baseColorOrder, "Warp Amp: Low (0.1)", nil, nil, true},
			// {baseAngle, baseWarpFreq, 0.5, baseWarpFreq, 0.5, baseHillFreq, baseHillAmp, baseColorOrder, "Warp Amp: Medium (0.5)", nil, nil, true}, // Same as medium freq test
			{baseAngle, baseWarpFreq, 0.9, baseWarpFreq, 0.9, baseHillFreq, baseHillAmp, baseColorOrder, "Warp Amp: High (0.9)", nil, nil, true},

		}},
		{"hill", []TestParameters{
			// Use isAblationOverride: true to force hillFreq/Amp via hash override
			{baseAngle, baseWarpFreq, baseWarpAmp, baseWarpFreq, baseWarpAmp, 0.1, 0.1, baseColorOrder, "Hill Freq/Amp: Low (0.1)", nil, nil, true},
			{baseAngle, baseWarpFreq, baseWarpAmp, baseWarpFreq, baseWarpAmp, 0.5, 0.5, baseColorOrder, "Hill Freq/Amp: Medium (0.5)", nil, nil, true},
			{baseAngle, baseWarpFreq, baseWarpAmp, baseWarpFreq, baseWarpAmp, 0.9, 0.9, baseColorOrder, "Hill Freq/Amp: High (0.9)", nil, nil, true},
		}},
		{"color_order", []TestParameters{
			// Use isAblationOverride: true to force colorOrder via hash override
			{baseAngle, baseWarpFreq, baseWarpAmp, baseWarpFreq, baseWarpAmp, baseHillFreq, baseHillAmp, 0, "Color Order 0 (R-W-G)", nil, nil, true},
			{baseAngle, baseWarpFreq, baseWarpAmp, baseWarpFreq, baseWarpAmp, baseHillFreq, baseHillAmp, 1, "Color Order 1 (R-G-W)", nil, nil, true},
			{baseAngle, baseWarpFreq, baseWarpAmp, baseWarpFreq, baseWarpAmp, baseHillFreq, baseHillAmp, 2, "Color Order 2 (W-R-G)", nil, nil, true},
			{baseAngle, baseWarpFreq, baseWarpAmp, baseWarpFreq, baseWarpAmp, baseHillFreq, baseHillAmp, 3, "Color Order 3 (W-G-R)", nil, nil, true},
			{baseAngle, baseWarpFreq, baseWarpAmp, baseWarpFreq, baseWarpAmp, baseHillFreq, baseHillAmp, 4, "Color Order 4 (G-R-W)", nil, nil, true},
			{baseAngle, baseWarpFreq, baseWarpAmp, baseWarpFreq, baseWarpAmp, baseHillFreq, baseHillAmp, 5, "Color Order 5 (G-W-R)", nil, nil, true},
		}},
		{"edge_cases", []TestParameters{
			// Use isAblationOverride: true to force edge parameters via hash override
			{baseAngle, 0.01, 0.05, 0.01, 0.05, baseHillFreq, baseHillAmp, baseColorOrder, "Edge: Near zero warp freq/amp", nil, nil, true},
			{baseAngle, 0.99, 0.95, 0.99, 0.95, baseHillFreq, baseHillAmp, baseColorOrder, "Edge: Near max warp freq/amp", nil, nil, true},
			{baseAngle, baseWarpFreq, baseWarpAmp, baseWarpFreq, baseWarpAmp, 0.01, 0.01, baseColorOrder, "Edge: Near zero hill freq/amp", nil, nil, true},
			{baseAngle, baseWarpFreq, baseWarpAmp, baseWarpFreq, baseWarpAmp, 0.99, 0.99, baseColorOrder, "Edge: Near max hill freq/amp", nil, nil, true},
			{0.0, baseWarpFreq, baseWarpAmp, baseWarpFreq, baseWarpAmp, baseHillFreq, baseHillAmp, baseColorOrder, "Edge: Zero angle", nil, nil, true},
			{baseAngle, 0.9, 0.8, 0.9, 0.8, 0.9, 0.9, baseColorOrder, "Edge: High warp & hill interaction", nil, nil, true},
		}},
		// --- New Test Cases ---
		{"input_string", []TestParameters{
			// isAblationOverride: false - let hash naturally determine parameters from string
			{0, 0, 0, 0, 0, 0, 0, 0, "Input: 'test' (Baseline)", nil, nil, false}, // Params ignored when false
			{0, 0, 0, 0, 0, 0, 0, 0, "Input: 'hi'", nil, &strShort, false},
			{0, 0, 0, 0, 0, 0, 0, 0, "Input: Long w/ Symbols", nil, &strLong, false},
			{0, 0, 0, 0, 0, 0, 0, 0, "Input: 'testing1'", nil, &strSamePrefix, false},
			{0, 0, 0, 0, 0, 0, 0, 0, "Input: 'testing2'", nil, &strSamePrefixDiff, false},
		}},
		{"palette", []TestParameters{
			// Use isAblationOverride: true to force base params, only palette changes
			{baseAngle, baseWarpFreq, baseWarpAmp, baseWarpFreq, baseWarpAmp, baseHillFreq, baseHillAmp, baseColorOrder, "Palette: Default (R/W/G)", nil, nil, true},
			{baseAngle, baseWarpFreq, baseWarpAmp, baseWarpFreq, baseWarpAmp, baseHillFreq, baseHillAmp, baseColorOrder, "Palette: Blue (Navy/Aqua/W)", &bluePalette, nil, true},
			{baseAngle, baseWarpFreq, baseWarpAmp, baseWarpFreq, baseWarpAmp, baseHillFreq, baseHillAmp, baseColorOrder, "Palette: Earth (Browns/Green)", &earthPalette, nil, true},
			// Example with different color order on a different palette
			{baseAngle, baseWarpFreq, baseWarpAmp, baseWarpFreq, baseWarpAmp, baseHillFreq, baseHillAmp, 3, "Palette: Blue, Order 3 (W-Aqua-Navy)", &bluePalette, nil, true},
		}},
		// --- Ablation Study Cases ---
		{"ablation", []TestParameters{
			// Use isAblationOverride: true to force specific params for comparison
			// Baseline: Linear Gradient (No Warp, No Hill)
			{baseAngle, baseWarpFreq, 0.0, baseWarpFreq, 0.0, baseHillFreq, 0.0, baseColorOrder, "Ablation: Linear Gradient", nil, nil, true},
			// Warp Only (Medium Freq/Amp, No Hill)
			{baseAngle, baseWarpFreq, baseWarpAmp, baseWarpFreq, baseWarpAmp, baseHillFreq, 0.0, baseColorOrder, "Ablation: Warp Only", nil, nil, true},
			// Hill Only (Medium Freq/Amp, No Warp)
			{baseAngle, baseWarpFreq, 0.0, baseWarpFreq, 0.0, baseHillFreq, baseHillAmp, baseColorOrder, "Ablation: Hill Only", nil, nil, true},
			// Standard (Medium everything - same as medium warp/hill tests) - for comparison
			{baseAngle, baseWarpFreq, baseWarpAmp, baseWarpFreq, baseWarpAmp, baseHillFreq, baseHillAmp, baseColorOrder, "Ablation: Standard (Warp+Hill)", nil, nil, true},
		}},
		// --- 2D Parameter Sweep Example: Warp Freq vs Amp ---
		{"warp_sweep_2d", []TestParameters{
			// Grid: Rows = Warp Freq (Low, Med, High), Cols = Warp Amp (Low, Med, High)
			// All use baseAngle, baseHillFreq, baseHillAmp, baseColorOrder
			// Low Freq Row
			{baseAngle, 0.1, 0.1, 0.1, 0.1, baseHillFreq, baseHillAmp, baseColorOrder, "Warp Freq=0.1, Amp=0.1", nil, nil, true}, // Freq=Low, Amp=Low
			{baseAngle, 0.1, 0.5, 0.1, 0.5, baseHillFreq, baseHillAmp, baseColorOrder, "Warp Freq=0.1, Amp=0.5", nil, nil, true}, // Freq=Low, Amp=Med
			{baseAngle, 0.1, 0.9, 0.1, 0.9, baseHillFreq, baseHillAmp, baseColorOrder, "Warp Freq=0.1, Amp=0.9", nil, nil, true}, // Freq=Low, Amp=High
			// Medium Freq Row
			{baseAngle, 0.5, 0.1, 0.5, 0.1, baseHillFreq, baseHillAmp, baseColorOrder, "Warp Freq=0.5, Amp=0.1", nil, nil, true}, // Freq=Med, Amp=Low
			{baseAngle, 0.5, 0.5, 0.5, 0.5, baseHillFreq, baseHillAmp, baseColorOrder, "Warp Freq=0.5, Amp=0.5", nil, nil, true}, // Freq=Med, Amp=Med (Standard)
			{baseAngle, 0.5, 0.9, 0.5, 0.9, baseHillFreq, baseHillAmp, baseColorOrder, "Warp Freq=0.5, Amp=0.9", nil, nil, true}, // Freq=Med, Amp=High
			// High Freq Row
			{baseAngle, 0.9, 0.1, 0.9, 0.1, baseHillFreq, baseHillAmp, baseColorOrder, "Warp Freq=0.9, Amp=0.1", nil, nil, true}, // Freq=High, Amp=Low
			{baseAngle, 0.9, 0.5, 0.9, 0.5, baseHillFreq, baseHillAmp, baseColorOrder, "Warp Freq=0.9, Amp=0.5", nil, nil, true}, // Freq=High, Amp=Med
			{baseAngle, 0.9, 0.9, 0.9, 0.9, baseHillFreq, baseHillAmp, baseColorOrder, "Warp Freq=0.9, Amp=0.9", nil, nil, true}, // Freq=High, Amp=High
		}},
	}

	var wg sync.WaitGroup
	totalTests := 0
	for _, tc := range testCases { totalTests += len(tc.params) }
	fmt.Printf("Starting %d parameter test cases...\n", totalTests)

	for _, testCase := range testCases {
		// Ensure the test type directory exists before launching goroutines for it
		dir := filepath.Join("tests", testCase.testType)
		if err := os.MkdirAll(dir, 0755); err != nil {
			log.Printf("Error creating directory %s: %v. Skipping tests in this category.", dir, err)
			continue // Skip this test category if dir creation fails
		}

		for i, params := range testCase.params {
			wg.Add(1)
			// Capture loop variables for the goroutine
			currentParams := params
			currentIndex := i
			currentTestType := testCase.testType
			go func() {
				defer wg.Done()
				// Call generateTestImage and get image, calculated params, and duration
				img, calcParams, duration := generateTestImage(currentParams)
				filename := fmt.Sprintf("test_%d.png", currentIndex)
				// Pass description, calcParams, and duration to saveTestImage
				if err := saveTestImage(img, currentTestType, filename, currentParams.description, calcParams, duration); err != nil {
					log.Printf("Error saving test %s/%s: %v", currentTestType, filename, err)
				}
			}()
		}
	}
	wg.Wait()
	fmt.Println("Parameter tests completed.")
}

// generateGradientImage creates a gradient image based on hash bytes and base colors
// Now accepts width, height, and useSmoothstep arguments
func generateGradientImage(hashBytes []byte, baseColor1, baseColor2, baseColor3 color.RGBA, width, height int, useSmoothstep bool) (*image.RGBA, map[string]float64) {
	calculatedParams := make(map[string]float64)
	// Use hash to determine gradient parameters (linear, warp, hill wave)

	// Bytes 0-7 (8 bytes) for Linear Gradient Angle
	angleSeed := float64(uint64(hashBytes[0])<<56|
		uint64(hashBytes[1])<<48|
		uint64(hashBytes[2])<<40|
		uint64(hashBytes[3])<<32|
		uint64(hashBytes[4])<<24|
		uint64(hashBytes[5])<<16|
		uint64(hashBytes[6])<<8|
		uint64(hashBytes[7])) / float64(math.MaxUint64)
	angle := angleSeed * 2 * math.Pi
	dx := math.Cos(angle)
	dy := math.Sin(angle)
	calculatedParams["angleSeed"] = angleSeed
	calculatedParams["angleDegrees"] = angle * 180 / math.Pi
	calculatedParams["dx"] = dx
	calculatedParams["dy"] = dy

	// --- Warp Parameters ---
	// Use provided width and height for diagonal calculation
	imgDiagonal := math.Sqrt(float64(width*width + height*height))
	calculatedParams["imgDiagonal"] = imgDiagonal

	// Bytes 8-11 (4 bytes) for Warp X Frequency
	warpFreqXSeed := float64(readUint32(hashBytes[8:12])) / float64(math.MaxUint32)
	warpFreqXCycles := warpFreqXSeed*0.5 + 0.25              // Target 0.25-0.75 cycles across diagonal
	warpFreqX := warpFreqXCycles * 2 * math.Pi / imgDiagonal // Scale freq by diagonal
	calculatedParams["warpFreqXSeed"] = warpFreqXSeed
	calculatedParams["warpFreqXCycles"] = warpFreqXCycles
	calculatedParams["warpFreqX"] = warpFreqX

	// Bytes 12-13 (2 bytes) for Warp X Amplitude
	warpAmpXSeed := float64(readUint16(hashBytes[12:14])) / float64(math.MaxUint16)
	warpAmpX := warpAmpXSeed * imgDiagonal * 0.2 // Scale amp by diagonal (adjust multiplier)
	if warpAmpXSeed == 0 { warpAmpX = 0 }
	calculatedParams["warpAmpXSeed"] = warpAmpXSeed
	calculatedParams["warpAmpX"] = warpAmpX

	// Bytes 14-15 (2 bytes) for Warp X Phase
	warpPhaseXSeed := float64(readUint16(hashBytes[14:16])) / float64(math.MaxUint16)
	warpPhaseX := warpPhaseXSeed * 2 * math.Pi
	calculatedParams["warpPhaseXSeed"] = warpPhaseXSeed
	calculatedParams["warpPhaseX"] = warpPhaseX

	// Bytes 16-19 (4 bytes) for Warp Y Frequency
	warpFreqYSeed := float64(readUint32(hashBytes[16:20])) / float64(math.MaxUint32)
	warpFreqYCycles := warpFreqYSeed*0.5 + 0.25              // Target 0.25-0.75 cycles across diagonal
	warpFreqY := warpFreqYCycles * 2 * math.Pi / imgDiagonal // Scale freq by diagonal
	calculatedParams["warpFreqYSeed"] = warpFreqYSeed
	calculatedParams["warpFreqYCycles"] = warpFreqYCycles
	calculatedParams["warpFreqY"] = warpFreqY

	// Bytes 20-21 (2 bytes) for Warp Y Amplitude
	warpAmpYSeed := float64(readUint16(hashBytes[20:22])) / float64(math.MaxUint16)
	warpAmpY := warpAmpYSeed * imgDiagonal * 0.2 // Scale amp by diagonal (adjust multiplier)
	if warpAmpYSeed == 0 { warpAmpY = 0 }
	calculatedParams["warpAmpYSeed"] = warpAmpYSeed
	calculatedParams["warpAmpY"] = warpAmpY

	// Bytes 22-23 (2 bytes) for Warp Y Phase
	warpPhaseYSeed := float64(readUint16(hashBytes[22:24])) / float64(math.MaxUint16)
	warpPhaseY := warpPhaseYSeed * 2 * math.Pi
	calculatedParams["warpPhaseYSeed"] = warpPhaseYSeed
	calculatedParams["warpPhaseY"] = warpPhaseY

	// --- Rolling Hill Wave Parameters (Applied After Warp) ---
	// Bytes 24-27 (4 bytes) for Hill Wave Frequency (already scaled by diagonal)
	hillFreqSeed := float64(readUint32(hashBytes[24:28])) / float64(math.MaxUint32)
	hillNumCycles := hillFreqSeed*0.5 + 0.25 // Target 0.25-0.75 cycles across diagonal
	hillFrequency := hillNumCycles * 2 * math.Pi / imgDiagonal
	hillFreqX := hillFrequency * dx // Wave frequency components based on main angle
	hillFreqY := hillFrequency * dy
	calculatedParams["hillFreqSeed"] = hillFreqSeed
	calculatedParams["hillNumCycles"] = hillNumCycles
	calculatedParams["hillFrequency"] = hillFrequency
	calculatedParams["hillFreqX"] = hillFreqX
	calculatedParams["hillFreqY"] = hillFreqY

	// Bytes 28-29 (2 bytes) for Hill Wave Phase Shift
	hillPhaseSeed := float64(readUint16(hashBytes[28:30])) / float64(math.MaxUint16)
	hillPhase := hillPhaseSeed * 2 * math.Pi
	calculatedParams["hillPhaseSeed"] = hillPhaseSeed
	calculatedParams["hillPhase"] = hillPhase

	// Byte 30 (1 byte) for Color Order
	orderIndex := int(hashBytes[30]) % 6 // 3! = 6 permutations
	calculatedParams["orderIndex"] = float64(orderIndex) // Store as float64 for map consistency

	// Byte 31 (1 byte) for Hill Wave Amplitude
	hillAmpSeed := float64(hashBytes[31]) / float64(math.MaxUint8)
	hillAmplitude := hillAmpSeed*0.25 + 0.05 // Map [0,1] to [0.05, 0.3]
	if hillAmpSeed == 0 { hillAmplitude = 0 }
	calculatedParams["hillAmpSeed"] = hillAmpSeed
	calculatedParams["hillAmplitude"] = hillAmplitude

	// Define the final colors based on the determined order using the provided base colors
	var cFirst, cMiddle, cLast color.RGBA
	switch orderIndex {
	case 0:
		cFirst, cMiddle, cLast = baseColor1, baseColor2, baseColor3 // R, W, G
	case 1:
		cFirst, cMiddle, cLast = baseColor1, baseColor3, baseColor2 // R, G, W
	case 2:
		cFirst, cMiddle, cLast = baseColor2, baseColor1, baseColor3 // W, R, G
	case 3:
		cFirst, cMiddle, cLast = baseColor2, baseColor3, baseColor1 // W, G, R
	case 4:
		cFirst, cMiddle, cLast = baseColor3, baseColor1, baseColor2 // G, R, W
	case 5:
		cFirst, cMiddle, cLast = baseColor3, baseColor2, baseColor1 // G, W, R
	}

	// Create image using provided dimensions
	img := image.NewRGBA(image.Rect(0, 0, width, height))

	// Determine the min/max projection values of the BASE LINEAR gradient (using corners)
	minProj, maxProj := math.MaxFloat64, -math.MaxFloat64
	corners := []struct{ x, y float64 }{
		{0, 0}, {float64(width), 0}, {0, float64(height)}, {float64(width), float64(height)},
	}
	for _, p := range corners {
		proj := p.x*dx + p.y*dy // Simple linear projection for range calculation
		minProj = math.Min(minProj, proj)
		maxProj = math.Max(maxProj, proj)
	}
	projRange := maxProj - minProj
	if projRange == 0 {
		projRange = 1
	} else {
		projRange *= 1.1 // Expand range slightly by 10%
	}

	// Pre-calculate stride for direct pixel access
	stride := img.Stride
	pixels := img.Pix

	// Fill pixels using coordinate warping with direct pixel access
	for y := 0; y < height; y++ {
		fy := float64(y)
		baseOffset := y * stride

		// Pre-calculate y-dependent values
		sinWarpY := math.Sin(warpFreqX*fy + warpPhaseX)
		dispX := warpAmpX * sinWarpY

		for x := 0; x < width; x++ {
			fx := float64(x)
			offset := baseOffset + x*4 // 4 bytes per pixel (RGBA)

			// Calculate displacement for Y based on x
			dispY := warpAmpY * math.Sin(warpFreqY*fx+warpPhaseY)

			// Calculate source coordinates by applying displacement
			srcX := fx + dispX
			srcY := fy + dispY

			// Calculate base gradient projection using SOURCE coordinates
			proj := srcX*dx + srcY*dy

			// Normalize the base projection value
			tBase := (proj - minProj) / projRange

			// Calculate "rolling hill" wave modification based on DESTINATION coordinates
			hillWaveArg := hillFreqX*fx + hillFreqY*fy + hillPhase
			tWave := 0.0
			if hillAmplitude > 0 { // Avoid unnecessary Sin calculation if amplitude is zero
				tWave = math.Sin(hillWaveArg)
			}

			// Combine base t and hill wave modification
			tFinalRaw := tBase + hillAmplitude*tWave // Value before clamping

			// Apply either standard clamping or smoothstep
			var tFinal float64
			if useSmoothstep {
				// Apply smoothstep mapping [0, 1] range smoothly
				tFinal = smoothstep(0.0, 1.0, tFinalRaw)
			} else {
				// Clamp the final value to [0, 1]
				tFinal = math.Max(0, math.Min(1, tFinalRaw))
			}

			// Interpolate color based on the final processed position 'tFinal'
			var r, g, b uint8
			if tFinal < 0.5 {
				// First half - interpolate between cFirst and cMiddle
				t := tFinal * 2
				r = uint8(float64(cFirst.R)*(1-t) + float64(cMiddle.R)*t)
				g = uint8(float64(cFirst.G)*(1-t) + float64(cMiddle.G)*t)
				b = uint8(float64(cFirst.B)*(1-t) + float64(cMiddle.B)*t)
			} else {
				// Second half - interpolate between cMiddle and cLast
				t := (tFinal - 0.5) * 2
				r = uint8(float64(cMiddle.R)*(1-t) + float64(cLast.R)*t)
				g = uint8(float64(cMiddle.G)*(1-t) + float64(cLast.G)*t)
				b = uint8(float64(cMiddle.B)*(1-t) + float64(cLast.B)*t)
			}

			// Set pixel values directly in the image buffer
			pixels[offset] = r
			pixels[offset+1] = g
			pixels[offset+2] = b
			pixels[offset+3] = 255 // Alpha
		}
	}

	return img, calculatedParams
}

// blendImagesParallel blends two images using parallel processing
// Assumes img1 and img2 have the same dimensions.
func blendImagesParallel(img1, img2 *image.RGBA) *image.RGBA {
	bounds := img1.Bounds()
	if bounds != img2.Bounds() {
		// Handle error or panic if dimensions don't match
		// For simplicity, we'll assume they match for now.
		log.Printf("Warning: blending images with different bounds!")
	}
	blendedImg := image.NewRGBA(bounds)
	stride := img1.Stride // Assumes stride is the same if bounds are the same
	pixels1 := img1.Pix
	pixels2 := img2.Pix
	pixelsOut := blendedImg.Pix

	// Calculate number of workers based on available CPU cores
	numWorkers := min(runtime.NumCPU(), bounds.Dy()) // Use height for partitioning
	if numWorkers <= 0 { numWorkers = 1 } // Ensure at least one worker

	// Create a wait group to synchronize workers
	var wg sync.WaitGroup
	wg.Add(numWorkers)

	// Calculate rows per worker
	rowsPerWorker := bounds.Dy() / numWorkers
	extraRows := bounds.Dy() % numWorkers

	// Launch workers
	for i := 0; i < numWorkers; i++ {
		startRow := i * rowsPerWorker
		endRow := startRow + rowsPerWorker
		if i == numWorkers-1 {
			endRow += extraRows // Add remaining rows to last worker
		}

		go func(start, end int) {
			defer wg.Done()
			// Use bounds.Dx() and bounds.Dy() which come from the image itself
			for y := start; y < end; y++ {
				baseOffset := y * stride
				// Process 4 pixels at a time for better cache utilization
				for x := 0; x < bounds.Dx(); x += 4 {
					endX := min(x+4, bounds.Dx())

					// Process up to 4 pixels in this iteration
					for px := x; px < endX; px++ {
						pxOffset := baseOffset + px*4
						// Use uint16 for intermediate calculations to prevent overflow
						pixelsOut[pxOffset] = uint8((uint16(pixels1[pxOffset]) + uint16(pixels2[pxOffset])) / 2)
						pixelsOut[pxOffset+1] = uint8((uint16(pixels1[pxOffset+1]) + uint16(pixels2[pxOffset+1])) / 2)
						pixelsOut[pxOffset+2] = uint8((uint16(pixels1[pxOffset+2]) + uint16(pixels2[pxOffset+2])) / 2)
						pixelsOut[pxOffset+3] = 255 // Alpha
					}
				}
			}
		}(startRow, endRow)
	}

	wg.Wait()
	return blendedImg
}

// generateGradientImageConcurrent generates two images concurrently using a worker pool
// Update to pass width and height
func generateGradientImageConcurrent(hashBytes1, hashBytes2 []byte, baseColor1, baseColor2, baseColor3 color.RGBA, width, height int) (*image.RGBA, *image.RGBA) {
	var img1, img2 *image.RGBA
	var wg sync.WaitGroup
	wg.Add(2)

	// Generate first image
	go func() {
		defer wg.Done()
		// Pass width and height
		img1, _ = generateGradientImage(hashBytes1, baseColor1, baseColor2, baseColor3, width, height, false)
	}()

	// Generate second image
	go func() {
		defer wg.Done()
		// Pass width and height
		img2, _ = generateGradientImage(hashBytes2, baseColor1, baseColor2, baseColor3, width, height, false)
	}()

	wg.Wait()
	return img1, img2
}

// saveImageOptimized saves an image with optimized PNG encoding
func saveImageOptimized(img image.Image, filename string) error {
	outFile, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer outFile.Close()

	// Use PNG encoder with default compression
	if err := png.Encode(outFile, img); err != nil {
		return fmt.Errorf("failed to encode PNG: %w", err)
	}

	return nil
}

// --- New Function for Landscape Image ---

func generateLandscapeImage() {
	fmt.Println("Generating landscape showcase gradient (1920x1080)...")

	landscapeWidth := 1920
	landscapeHeight := 1080
	inputString := "LandscapeShowcase"
	outputDir := "paper_assets"
	outputBaseName := "landscape_gradient"

	// Create output directory
	err := os.MkdirAll(outputDir, 0755)
	if err != nil {
		log.Fatalf("Failed to create output directory '%s': %v", outputDir, err)
	}

	// Define the desired fixed parameters
	forcedParams := TestParameters{
		angleSeed:    0.1,   // Mostly horizontal flow
		warpFreqX:    0.6, 
		warpAmpX:     0.5, 
		warpFreqY:    0.6,
		warpAmpY:     0.5,
		hillFreq:     0.5,
		hillAmp:      0.2,
		colorOrder:   0,     // Default order
		description:  "Forced Landscape Showcase",
		isAblationOverride: true, // Ensure parameters are forced
	}

	// Define the color palette (Ocean)
	paletteHex := []string{"#0096FF", "#6CBCFC", "#A1D6FF"}
	var baseColors [3]color.RGBA
	for i, hex := range paletteHex {
		baseColors[i], err = hexToRGBA(hex)
		if err != nil {
			log.Fatalf("Invalid hex color '%s': %v", hex, err)
		}
	}

	// 1. Generate hash for the input string
	hasher := sha256.New()
	hasher.Write([]byte(inputString))
	hashBytes := hasher.Sum(nil)

	// 2. Create modified hash bytes using the override logic
	// (This logic is similar to the start of generateTestImage,
	// we could extract it to a helper if used often)
	modifiedHashBytes := make([]byte, len(hashBytes))
	copy(modifiedHashBytes, hashBytes) // Start with original hash

	if forcedParams.isAblationOverride { // Apply overrides
		// Angle Seed (Bytes 0-7)
		// Calculate the uint64 representation corresponding to the desired float64 seed [0, 1)
		angleSeedUint64 := uint64(forcedParams.angleSeed * float64(math.MaxUint64))
		binary.BigEndian.PutUint64(modifiedHashBytes[0:8], angleSeedUint64)

		// Warp X Freq (Bytes 8-11)
		// Note: Other overrides might have similar issues if the target value is float-based

		// Warp X Amp (Bytes 12-13)
		binary.BigEndian.PutUint16(modifiedHashBytes[12:14], uint16(forcedParams.warpAmpX*float64(math.MaxUint16)))

		// Warp Y Freq (Bytes 16-19)
		binary.BigEndian.PutUint32(modifiedHashBytes[16:20], uint32(forcedParams.warpFreqY*float64(math.MaxUint32)))
		// Warp Y Amp (Bytes 20-21)
		binary.BigEndian.PutUint16(modifiedHashBytes[20:22], uint16(forcedParams.warpAmpY*float64(math.MaxUint16)))

		// Hill Freq (Bytes 24-27)
		binary.BigEndian.PutUint32(modifiedHashBytes[24:28], uint32(forcedParams.hillFreq*float64(math.MaxUint32)))
		// Hill Amp (Byte 31)
		modifiedHashBytes[31] = byte(forcedParams.hillAmp * float64(math.MaxUint8))
		// Color Order (Byte 30)
		modifiedHashBytes[30] = byte(forcedParams.colorOrder)
		// Note: Phases (Bytes 14-15, 22-23, 28-29) are not overridden here, 
		// they will still come from the original hash, adding some variation.
	}
	
	// 3. Generate *two* images using the *same* modified hash bytes and landscape dimensions
	// This ensures both images in the blend use the forced parameters.
	fmt.Println("Generating landscape images...")
	startTime := time.Now()
	img1, calcParams1 := generateGradientImage(modifiedHashBytes, baseColors[0], baseColors[1], baseColors[2], landscapeWidth, landscapeHeight, false)
	// For the second image, we could use hash(reversed(inputString)) + override,
	// but using the exact same modified hash simplifies this showcase example.
	img2, _ := generateGradientImage(modifiedHashBytes, baseColors[0], baseColors[1], baseColors[2], landscapeWidth, landscapeHeight, false)
	genDuration := time.Since(startTime)
	fmt.Printf("Image generation took: %s\n", genDuration)

	// 4. Blend the images
	fmt.Println("Blending landscape images...")
	blendedImg := blendImagesParallel(img1, img2)

	// 5. Save the blended image
	imgOutputPath := filepath.Join(outputDir, outputBaseName+".png")
	fmt.Println("Saving landscape image...")
	if err := saveImageOptimized(blendedImg, imgOutputPath); err != nil {
		log.Fatalf("Failed to save landscape image: %v", err)
	}

	// 6. Save parameters to a text file
	txtOutputPath := filepath.Join(outputDir, outputBaseName+".txt")
	descFile, err := os.Create(txtOutputPath)
	if err != nil {
		log.Fatalf("Failed to create landscape description file: %v", err)
	}
	defer descFile.Close()

	_, _ = fmt.Fprintf(descFile, "Landscape Showcase Gradient\n")
	_, _ = fmt.Fprintf(descFile, "Source String: %s\n", inputString)
	_, _ = fmt.Fprintf(descFile, "Dimensions: %d x %d\n", landscapeWidth, landscapeHeight)
	_, _ = fmt.Fprintf(descFile, "Generation Duration: %s\n\n", genDuration)
	
	_, _ = fmt.Fprintf(descFile, "--- Forced Parameters (Input Seeds) ---\n")
	_, _ = fmt.Fprintf(descFile, "angleSeed: %.6f\n", forcedParams.angleSeed)
	_, _ = fmt.Fprintf(descFile, "warpFreqX: %.6f\n", forcedParams.warpFreqX)
	_, _ = fmt.Fprintf(descFile, "warpAmpX:  %.6f\n", forcedParams.warpAmpX)
	_, _ = fmt.Fprintf(descFile, "warpFreqY: %.6f\n", forcedParams.warpFreqY)
	_, _ = fmt.Fprintf(descFile, "warpAmpY:  %.6f\n", forcedParams.warpAmpY)
	_, _ = fmt.Fprintf(descFile, "hillFreq:  %.6f\n", forcedParams.hillFreq)
	_, _ = fmt.Fprintf(descFile, "hillAmp:   %.6f\n", forcedParams.hillAmp)
	_, _ = fmt.Fprintf(descFile, "colorOrder: %d\n\n", forcedParams.colorOrder)
	
	_, _ = fmt.Fprintf(descFile, "--- Colors Used (Ocean Palette) ---\n")
	for _, hex := range paletteHex {
		_, _ = fmt.Fprintf(descFile, "- %s\n", hex)
	}
	_, _ = fmt.Fprintf(descFile, "\n")

	_, _ = fmt.Fprintf(descFile, "--- Calculated Parameters (from first image gen) ---\n")
	// Sort keys for consistent output order
    keys := make([]string, 0, len(calcParams1))
    for k := range calcParams1 {
        keys = append(keys, k)
    }
    sort.Strings(keys)
    for _, k := range keys {
        _, err = fmt.Fprintf(descFile, "%s: %.6f\n", k, calcParams1[k])
        if err != nil {
            log.Printf("Warning: failed to write calc param %s: %v", k, err)
        }
    }

	fmt.Printf("Successfully generated landscape gradient to %s (and .txt)\n", imgOutputPath)
}

// Helper function for smoothstep interpolation
// Maps a value x from edge0 to edge1 smoothly into the range [0, 1]
// Returns 0 if x <= edge0, 1 if x >= edge1
func smoothstep(edge0, edge1, x float64) float64 {
	// Scale, bias and saturate x to 0..1 range
	t := math.Max(0, math.Min(1, (x-edge0)/(edge1-edge0)))
	// Evaluate polynomial
	return t * t * (3 - 2*t)
}

func main() {
	// Check for special mode arguments *before* flag parsing
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "--test":
			runParameterTests()
			return
		// case "--japanese":
		// 	generateJapaneseGradient()
		// 	return
		// case "--video":
		// 	generateGradientVideo()
		// 	return
		case "--landscape":
			generateLandscapeImage()
			return
		// case "--clamping_test": // Removed - Logic moved to main_test.go
		// 	generateClampingTestImages()
		// 	return
		}
	}

	// --- Default behavior: Generate single image --- 

	// Define flags for default mode
	outputFilename := flag.String("output", "output.png", "Output filename for the generated image")
	
	// Parse flags for default mode
	flag.Parse()

	// Use flag.Args() to get the input string for default mode
	args := flag.Args()

	// In default mode, exactly one non-flag argument (the input string) is expected
	if len(args) != 1 {
		fmt.Fprintf(os.Stderr, "Usage for default mode: %s [options] <input_string>\n", os.Args[0])
		// fmt.Fprintf(os.Stderr, "Usage for special modes: %s <--test | --landscape | --clamping_test>\nOptions for default mode:\n", os.Args[0]) // Updated usage message
		fmt.Fprintf(os.Stderr, "Usage for special modes: %s <--test | --landscape>\nOptions for default mode:\n", os.Args[0])
		flag.PrintDefaults()
		os.Exit(1)
	}

	inputString := args[0] // Input string for default mode

	// // Original switch logic - now handled before flag parsing for modes
	// switch modeOrInput {
	// case "--test":
	// 	runParameterTests()
	// 	return
	// // case "--japanese":
	// // 	generateJapaneseGradient()
	// // 	return
	// // case "--video":
	// // 	generateGradientVideo()
	// // 	return
	// case "--landscape":
	// 	generateLandscapeImage()
	// 	return
	// case "--clamping_test": // Removed
	// 	generateClampingTestImages()
	// 	return
	// }

	// Default behavior: generate single image from input string
	// inputString := modeOrInput // Assume it's the input string if not a special mode
	// // Check if any other non-flag args were provided unexpectedly
	// if len(args) > 1 {
	// 	fmt.Fprintf(os.Stderr, "Warning: Extra arguments provided after input string: %v\n", args[1:])
	// }

	// --- Proceed with default image generation logic --- 

	reversedString := reverseString(inputString)

	// 1. Define base colors (used for both images) - Default palette
	baseColor1, err := hexToRGBA(defaultPalette[0]) 
	if err != nil {
		log.Fatal(err)
	}
	baseColor2, err := hexToRGBA(defaultPalette[1]) 
	if err != nil {
		log.Fatal(err)
	}
	baseColor3, err := hexToRGBA(defaultPalette[2]) 
	if err != nil {
		log.Fatal(err)
	}

	// 2. Generate hashes for both strings
	hasher1 := sha256.New()
	hasher1.Write([]byte(inputString))
	hashBytes1 := hasher1.Sum(nil)

	hasher2 := sha256.New()
	hasher2.Write([]byte(reversedString))
	hashBytes2 := hasher2.Sum(nil)

	// 3. Generate both images concurrently using global dimensions
	fmt.Println("Generating images concurrently...")
	img1, img2 := generateGradientImageConcurrent(hashBytes1, hashBytes2, baseColor1, baseColor2, baseColor3, imgWidth, imgHeight)

	// 4. Blend the images in parallel
	fmt.Println("Blending images in parallel...")
	blendedImg := blendImagesParallel(img1, img2)

	// 5. Save the blended image with optimized encoding
	fmt.Println("Saving image with optimized encoding...")
	// Use the output filename from the flag
	if err := saveImageOptimized(blendedImg, *outputFilename); err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Successfully generated blended %s\n", *outputFilename)
}
