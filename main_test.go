package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"math"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

func TestHexToRGBA(t *testing.T) {
	testCases := []struct {
		name     string
		hex      string
		expected color.RGBA
		wantErr  bool
	}{
		{"Valid 6-digit", "#FF0000", color.RGBA{R: 255, G: 0, B: 0, A: 255}, false},
		{"Valid 3-digit", "#0F0", color.RGBA{R: 0, G: 255, B: 0, A: 255}, false},
		{"White 6-digit", "#FFFFFF", color.RGBA{R: 255, G: 255, B: 255, A: 255}, false},
		{"White 3-digit", "#FFF", color.RGBA{R: 255, G: 255, B: 255, A: 255}, false},
		{"Black 6-digit", "#000000", color.RGBA{R: 0, G: 0, B: 0, A: 255}, false},
		{"Black 3-digit", "#000", color.RGBA{R: 0, G: 0, B: 0, A: 255}, false},
		{"Mixed Case", "#fA8072", color.RGBA{R: 250, G: 128, B: 114, A: 255}, false}, // Salmon
		{"Invalid Length 5", "#12345", color.RGBA{}, true},
		{"Invalid Length 7", "#1234567", color.RGBA{}, true},
		{"Invalid Chars", "#GG0000", color.RGBA{}, true},
		{"Missing Hash", "FF0000", color.RGBA{}, true},
		{"Empty String", "", color.RGBA{}, true},
		{"Just Hash", "#", color.RGBA{}, true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			rgba, err := hexToRGBA(tc.hex)
			if (err != nil) != tc.wantErr {
				t.Errorf("hexToRGBA(%q) error = %v, wantErr %v", tc.hex, err, tc.wantErr)
				return
			}
			if !tc.wantErr && rgba != tc.expected {
				t.Errorf("hexToRGBA(%q) = %v, want %v", tc.hex, rgba, tc.expected)
			}
		})
	}
}

// Helper function for comparing floats with tolerance
func floatsAlmostEqual(a, b, tolerance float64) bool {
	return math.Abs(a-b) < tolerance
}

// TestParameterDerivation verifies that hash bytes are correctly mapped to parameter seeds.
func TestParameterDerivation(t *testing.T) {
	// Define a fixed, arbitrary 32-byte hash input for testing
	// Example: Bytes 0x01, 0x02, ..., 0x20 (32 bytes)
	hashInput := []byte{
		0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, // angleSeed bytes (0-7)
		0x09, 0x0A, 0x0B, 0x0C,                     // warpFreqXSeed bytes (8-11)
		0x0D, 0x0E,                                 // warpAmpXSeed bytes (12-13)
		0x0F, 0x10,                                 // warpPhaseXSeed bytes (14-15)
		0x11, 0x12, 0x13, 0x14,                     // warpFreqYSeed bytes (16-19)
		0x15, 0x16,                                 // warpAmpYSeed bytes (20-21)
		0x17, 0x18,                                 // warpPhaseYSeed bytes (22-23)
		0x19, 0x1A, 0x1B, 0x1C,                     // hillFreqSeed bytes (24-27)
		0x1D, 0x1E,                                 // hillPhaseSeed bytes (28-29)
		0x1F,                                       // orderIndexSeed byte (30)
		0x20,                                       // hillAmpSeed byte (31)
	}

	// Manually calculate expected normalized seeds (0-1 range) based on hashInput
	expectedSeeds := map[string]float64{
		"angleSeed":      float64(binary.BigEndian.Uint64(hashInput[0:8])) / float64(math.MaxUint64),
		"warpFreqXSeed":  float64(binary.BigEndian.Uint32(hashInput[8:12])) / float64(math.MaxUint32),
		"warpAmpXSeed":   float64(binary.BigEndian.Uint16(hashInput[12:14])) / float64(math.MaxUint16),
		"warpPhaseXSeed": float64(binary.BigEndian.Uint16(hashInput[14:16])) / float64(math.MaxUint16),
		"warpFreqYSeed":  float64(binary.BigEndian.Uint32(hashInput[16:20])) / float64(math.MaxUint32),
		"warpAmpYSeed":   float64(binary.BigEndian.Uint16(hashInput[20:22])) / float64(math.MaxUint16),
		"warpPhaseYSeed": float64(binary.BigEndian.Uint16(hashInput[22:24])) / float64(math.MaxUint16),
		"hillFreqSeed":   float64(binary.BigEndian.Uint32(hashInput[24:28])) / float64(math.MaxUint32),
		"hillPhaseSeed":  float64(binary.BigEndian.Uint16(hashInput[28:30])) / float64(math.MaxUint16),
		"orderIndex":     float64(int(hashInput[30]) % 6), // Note: Testing the final index, not the raw seed
		"hillAmpSeed":    float64(hashInput[31]) / float64(math.MaxUint8),
	}

	// Call generateGradientImage with the test hash and dummy colors
	// We only care about the returned calculatedParams map
	dummyColor := color.RGBA{R: 0, G: 0, B: 0, A: 255}
	_, calculatedParams := generateGradientImage(hashInput, dummyColor, dummyColor, dummyColor, imgWidth, imgHeight, false)

	// Define tolerance for float comparison
	tolerance := 1e-9

	// Compare each expected seed against the calculated parameters
	for name, expected := range expectedSeeds {
		actual, ok := calculatedParams[name]
		if !ok {
			t.Errorf("Parameter %s not found in calculatedParams map", name)
			continue
		}
		
		// Use specific check for orderIndex as it's discrete
		if name == "orderIndex" {
			if uint8(actual) != uint8(expected) {
				t.Errorf("Parameter %s mismatch: got %.0f, want %.0f", name, actual, expected)
			}
		} else if !floatsAlmostEqual(actual, expected, tolerance) {
			t.Errorf("Parameter %s mismatch: got %.15f, want %.15f", name, actual, expected)
		}
	}

	// Check if any unexpected parameters were calculated (optional, but good practice)
	// for name := range calculatedParams {
	// 	if _, expected := expectedSeeds[name]; !expected && name[0] != '_' { // Ignore internal _input params
	// 		t.Errorf("Unexpected parameter calculated: %s", name)
	// 	}
	// }
}

// TestDeterminism checks if generating an image twice with the same input yields identical pixel data.
func TestDeterminism(t *testing.T) {
	// Use a fixed input string and default parameters for the test
	// Note: We use generateTestImage as it encapsulates hash generation and parameter application
	// We choose parameters that involve all calculation paths (warp, hill).
	// Setting isAblationOverride=false ensures the process starts from the string hash.
	testParams := TestParameters{
		inputStr:           PtrToString("deterministic_test_string"),
		angleSeed:          0, // These values are ignored when isAblationOverride is false
		warpFreqX:          0, 
		warpAmpX:           0,
		warpFreqY:          0,
		warpAmpY:           0,
		hillFreq:           0,
		hillAmp:            0,
		colorOrder:         0,
		description:        "Determinism Test",
		palette:            nil, // Use default palette
		isAblationOverride: false, // Crucial: Ensure hash is derived from inputStr
	}

	// Generate the first image
	img1, _, duration1 := generateTestImage(testParams)
	t.Logf("Generated first image in %s", duration1)

	// Generate the second image using the exact same parameters
	// (Crucially, using the same testParams instance ensures identical inputs)
	img2, _, duration2 := generateTestImage(testParams)
	t.Logf("Generated second image in %s", duration2)

	// Compare dimensions first (should always be the same, but good sanity check)
	if img1.Bounds() != img2.Bounds() {
		t.Fatalf("Image dimensions differ: img1 %v, img2 %v", img1.Bounds(), img2.Bounds())
	}

	// Compare pixel data using SHA-256 checksums of the Pix slices
	hasher1 := sha256.New()
	hasher1.Write(img1.Pix)
	checksum1 := hasher1.Sum(nil)

	hasher2 := sha256.New()
	hasher2.Write(img2.Pix)
	checksum2 := hasher2.Sum(nil)

	if !bytes.Equal(checksum1, checksum2) {
		t.Errorf("Pixel data checksums do not match! Image generation is not deterministic for input: %s", *testParams.inputStr)
		t.Logf("Checksum 1: %x", checksum1)
		t.Logf("Checksum 2: %x", checksum2)
		// Optionally save the images for inspection upon failure
		// saveImageOptimized(img1, "debug_deterministic_fail_img1.png")
		// saveImageOptimized(img2, "debug_deterministic_fail_img2.png")
	}
}

// Helper function to get a pointer to a string (needed for TestParameters.inputStr)
func PtrToString(s string) *string {
	return &s
}

// --- Clamping Test Logic (Moved from main.go) ---

// generateClampingTestImages generates images to compare clamping effects.
// Note: This function logs directly and saves files, intended for visual inspection.
func generateClampingTestImages(t *testing.T) {
	t.Log("Generating clamping comparison images...")

	outputDir := "tests/clamping_comparison"
	err := os.MkdirAll(outputDir, 0755)
	if err != nil {
		t.Fatalf("Failed to create clamping test directory '%s': %v", outputDir, err)
	}

	// Define parameters designed to cause large tFinal swings
	forcedParams := TestParameters{
		angleSeed:    0.7,
		warpFreqX:    0.8,
		warpAmpX:     0.6,
		warpFreqY:    0.8,
		warpAmpY:     0.6,
		hillFreq:     0.9,
		hillAmp:      0.95, // VERY High hill amplitude seed
		colorOrder:   0,
		description:  "Clamping Artifact Test Parameters",
		isAblationOverride: true,
	}

	// Use a distinct palette (e.g., Earth)
	paletteHex := earthPalette // Assumes earthPalette is accessible or defined in test scope
	var baseColors [3]color.RGBA
	for i, hex := range paletteHex {
		baseColors[i], err = hexToRGBA(hex)
		if err != nil {
			t.Fatalf("Invalid hex color '%s': %v", hex, err)
		}
	}

	// Generate hash for a base string
	hasher := sha256.New()
	hasher.Write([]byte("ClampingTestString"))
	hashBytes := hasher.Sum(nil)

	// Create modified hash bytes using the override logic
	modifiedHashBytes := make([]byte, len(hashBytes))
	copy(modifiedHashBytes, hashBytes)

	if forcedParams.isAblationOverride {
		// Note: Direct float64 to Uint64 conversion like this isn't ideal for seed overrides.
		// It might not perfectly match the original intent if the input seeds were scaled.
		// However, for the purpose of this *specific* clamping test, it should be sufficient
		// to create conditions that highlight clamping differences.
		binary.BigEndian.PutUint64(modifiedHashBytes[0:8], math.Float64bits(forcedParams.angleSeed))
		binary.BigEndian.PutUint32(modifiedHashBytes[8:12], uint32(forcedParams.warpFreqX*float64(math.MaxUint32)))
		binary.BigEndian.PutUint16(modifiedHashBytes[12:14], uint16(forcedParams.warpAmpX*float64(math.MaxUint16)))
		binary.BigEndian.PutUint32(modifiedHashBytes[16:20], uint32(forcedParams.warpFreqY*float64(math.MaxUint32)))
		binary.BigEndian.PutUint16(modifiedHashBytes[20:22], uint16(forcedParams.warpAmpY*float64(math.MaxUint16)))
		binary.BigEndian.PutUint32(modifiedHashBytes[24:28], uint32(forcedParams.hillFreq*float64(math.MaxUint32)))
		modifiedHashBytes[31] = byte(forcedParams.hillAmp * float64(math.MaxUint8))
		modifiedHashBytes[30] = byte(forcedParams.colorOrder)
	}

	// Generate Image 1: Standard Clamping
	t.Log("Generating image with standard clamp...")
	imgClamp, paramsClamp := generateGradientImage(modifiedHashBytes, baseColors[0], baseColors[1], baseColors[2], imgWidth, imgHeight, false)
	imgClampPath := filepath.Join(outputDir, "clamping_standard.png")
	if err := saveImageOptimized(imgClamp, imgClampPath); err != nil {
		t.Fatalf("Failed to save standard clamp image: %v", err)
	}
	saveParamsToFile(imgClampPath, "Standard Clamping", forcedParams, paramsClamp, t) // Pass t for logging warnings
	t.Logf("Saved standard clamp image to %s", imgClampPath)

	// Generate Image 2: Smoothstep Clamping
	t.Log("Generating image with smoothstep...")
	imgSmooth, paramsSmooth := generateGradientImage(modifiedHashBytes, baseColors[0], baseColors[1], baseColors[2], imgWidth, imgHeight, true)
	imgSmoothPath := filepath.Join(outputDir, "clamping_smoothstep.png")
	if err := saveImageOptimized(imgSmooth, imgSmoothPath); err != nil {
		t.Fatalf("Failed to save smoothstep image: %v", err)
	}
	saveParamsToFile(imgSmoothPath, "Smoothstep Clamping", forcedParams, paramsSmooth, t) // Pass t for logging warnings
	t.Logf("Saved smoothstep image to %s", imgSmoothPath)

	t.Log("Clamping comparison images generated.")
}

// saveParamsToFile saves parameters (similar to parts of saveTestImage)
// Added *testing.T for logging errors/warnings within tests.
func saveParamsToFile(imagePath, testType string, inputParams TestParameters, calcParams map[string]float64, t *testing.T) {
	txtPath := imagePath + ".txt"
	descFile, err := os.Create(txtPath)
	if err != nil {
		t.Logf("Warning: Failed to create param file %s: %v", txtPath, err)
		return
	}
	defer descFile.Close()

	_, _ = fmt.Fprintf(descFile, "Test Description: %s\n", testType)
	_, _ = fmt.Fprintf(descFile, "Source Image: %s\n\n", filepath.Base(imagePath))

	_, _ = fmt.Fprintf(descFile, "--- Forced Parameters (Input Seeds) ---\n")
	_, _ = fmt.Fprintf(descFile, "angleSeed: %.6f\n", inputParams.angleSeed)
	_, _ = fmt.Fprintf(descFile, "warpFreqX: %.6f\n", inputParams.warpFreqX)
	_, _ = fmt.Fprintf(descFile, "warpAmpX:  %.6f\n", inputParams.warpAmpX)
	_, _ = fmt.Fprintf(descFile, "warpFreqY: %.6f\n", inputParams.warpFreqY)
	_, _ = fmt.Fprintf(descFile, "warpAmpY:  %.6f\n", inputParams.warpAmpY)
	_, _ = fmt.Fprintf(descFile, "hillFreq:  %.6f\n", inputParams.hillFreq)
	_, _ = fmt.Fprintf(descFile, "hillAmp:   %.6f\n", inputParams.hillAmp)
	_, _ = fmt.Fprintf(descFile, "colorOrder: %d\n\n", inputParams.colorOrder)

	_, _ = fmt.Fprintf(descFile, "--- Calculated Parameters ---\n")
    keys := make([]string, 0, len(calcParams))
    for k := range calcParams {
        keys = append(keys, k)
    }
    sort.Strings(keys)
    for _, k := range keys {
        _, err = fmt.Fprintf(descFile, "%s: %.6f\n", k, calcParams[k])
        if err != nil {
            // Log error within the test context instead of returning
            t.Logf("Warning: Failed to write calculated param %s to %s: %v", k, txtPath, err)
        }
    }
}

// TestGenerateClampingComparisonImages serves as the entry point for the clamping test.
func TestGenerateClampingComparisonImages(t *testing.T) {
	// This test primarily generates files for visual inspection.
	// We run the generation logic here.
	generateClampingTestImages(t)
	// No specific pass/fail assertions here, relies on visual check / no panics.
}

// --- Benchmarks ---

// BenchmarkGenerateGradientImage measures performance of the core image generation.
func BenchmarkGenerateGradientImage(b *testing.B) {
	// Setup: Create some fixed hash bytes and colors
	hashBytes := sha256.Sum256([]byte("benchmark-seed"))
	color1, _ := hexToRGBA("#FF0000")
	color2, _ := hexToRGBA("#FFFFFF")
	color3, _ := hexToRGBA("#00FF00")
	b.ResetTimer() // Start timing after setup
	for i := 0; i < b.N; i++ {
		_, _ = generateGradientImage(hashBytes[:], color1, color2, color3, imgWidth, imgHeight, false)
	}
}

// BenchmarkBlendImagesParallel measures performance of the parallel blending.
func BenchmarkBlendImagesParallel(b *testing.B) {
	// Setup: Create two dummy images of standard size
	rect := image.Rect(0, 0, imgWidth, imgHeight)
	img1 := image.NewRGBA(rect)
	img2 := image.NewRGBA(rect)
	// Optional: Fill with some data if needed, but for blending logic, size matters most.
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = blendImagesParallel(img1, img2)
	}
}

// BenchmarkFullProcessDefault measures the end-to-end default generation process.
func BenchmarkFullProcessDefault(b *testing.B) {
	inputString := "benchmark-process"
	reversedString := reverseString(inputString)
	color1, _ := hexToRGBA(defaultPalette[0])
	color2, _ := hexToRGBA(defaultPalette[1])
	color3, _ := hexToRGBA(defaultPalette[2])
	hasher1 := sha256.New()
	hasher1.Write([]byte(inputString))
	hashBytes1 := hasher1.Sum(nil)
	hasher2 := sha256.New()
	hasher2.Write([]byte(reversedString))
	hashBytes2 := hasher2.Sum(nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Simulate the core logic of main()
		img1, img2 := generateGradientImageConcurrent(hashBytes1, hashBytes2, color1, color2, color3, imgWidth, imgHeight)
		_ = blendImagesParallel(img1, img2)
		// Note: Saving is not benchmarked here as it involves I/O.
	}
} 