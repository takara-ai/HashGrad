<img src="https://takara.ai/images/logo-24/TakaraAi.svg" width="200" alt="Takara.ai Logo" />

From the Frontier Research Team at **Takara.ai** we present HashGrad, a system for generating visually rich colour fields from arbitrary strings using SHA-256 hashing.

This repository contains the testing framework and demo implementation for the paper:  
[**HashGrad: Deterministic Visual Parameter Mapping for Reproducible Gradient Synthesis in AI Training**](https://takara.ai/papers/HashGrad.pdf)

---

This program generates abstract gradient images based on an input text string using SHA-256 hashing to determine various visual parameters like gradient angle, color order, warp effects, and hill waves. It blends two images (one from the input string, one from its reverse) for the final output.

## Features

- Generates unique gradient images from any text input.
- Uses SHA-256 hashing for deterministic parameter generation.
- Includes effects like coordinate warping and rolling hill waves.
- Blends images generated from the input string and its reverse.
- Parallel processing for image generation and blending.
- Customizable color palettes (though the default mode uses a fixed red/white/gray palette).
- Special modes for generating test patterns and specific showcases.

## Prerequisites

- Go programming language (version 1.18 or later recommended).
- Python 3 (for analysis/visualization scripts).
- Python packages: Install using `pip install -r requirements.txt`.

## Usage

### Generating a Gradient Image

To generate a gradient image from a text string, run:

```bash
go run main.go "your text here"
```

This will create an `output.png` file in the current directory.

To specify a different output filename:

```bash
go run main.go -output my_gradient.png "some other text"
```

### Running Tests

The project includes unit tests (`main_test.go`) to verify core functionality like color conversion, parameter derivation from hashes, and determinism. It also now includes a test that generates images for visually comparing standard clamping vs. smoothstep interpolation.

To run all tests (including the clamping comparison image generation):

```bash
go test
```

For more detailed output, use the verbose flag:

```bash
go test -v
```

The tests will generate images in the `tests/clamping_comparison/` subdirectory for the smoothstep comparison. Other tests defined in `main.go` (like ablation studies or parameter sweeps invoked via `go run main.go --test`) might also generate images in other `tests/` subdirectories.

### Running Performance Benchmarks

Performance benchmarks are included in `main_test.go` to measure the speed of key operations. To run the benchmarks:

```bash
go test -bench=.
```

This command runs only the benchmark functions. To run both tests and benchmarks:

```bash
go test -bench=. -v
```

### Special Modes

The program has special modes triggered by command-line arguments:

- `go run main.go --landscape`: Generates a specific 1920x1080 landscape gradient image (`paper_assets/landscape_gradient.png`) using forced parameters and an ocean-like palette.
- `go run main.go --test`: Runs a series of parameter variation tests defined in `main.go`, generating multiple images in the `tests/` directory (e.g., `tests/angle/`, `tests/warp/`, etc.). This is different from `go test` which runs unit tests and benchmarks.

### Analysis & Visualization Scripts (Python)

Several Python scripts are included for analysis and visualization purposes. Ensure you have installed the required packages first (`pip install -r scripts/requirements.txt`).

- **`scripts/visual_collision_analyzer.py`**:

  - Generates images for many random strings using the Go program.
  - Calculates perceptual hashes (`phash`, `dhash`) for each image.
  - Analyzes exact and near hash collisions to estimate visual uniqueness.
  - **Requires a compiled Go binary.** Compile using `go build` first.
  - Usage: `python scripts/visual_collision_analyzer.py [options]`
  - Options:
    - `--num_samples N`: Number of random strings to test (default: 1000).
    - `--go_executable PATH`: Path to the compiled `txt-gradient` binary (default: `./txt-gradient`).
    - `--summary_file PATH`: Path to save the analysis summary (default: `analysis/collisions/visual_collision_summary.txt`, gitignored).
    - `--max_near_distance N`: Max Hamming distance for near collision (default: 4).
    - `--temp_dir PATH`: Directory for temporary images (default: system temp).

- **`scripts/param_complementarity_analyzer.py`**:

  - Generates many random strings and their reverses.
  - Calculates SHA-256 hashes for both.
  - Extracts parameter seeds from both hashes.
  - Calculates the Pearson correlation between seeds from original vs. reversed hashes for each parameter type.
  - Outputs results to the console and saves them to `analysis/complementarity/parameter_correlations.txt` (gitignored).
  - Usage: `python scripts/param_complementarity_analyzer.py`

- **`scripts/generate_noise.py`**:

  - Generates a simple 800x600 grayscale random noise image.
  - Saves it to `tests/baselines/random_noise.png` (gitignored).
  - Useful as a baseline for comparison (e.g., in FFT analysis).
  - Usage: `python scripts/generate_noise.py`

- **`scripts/fft_analyzer.py`**:
  - Performs a 2D Fast Fourier Transform (FFT) analysis on a single input image.
  - Applies a Hann window before FFT to reduce edge artifacts.
  - Saves the log magnitude spectrum (grayscale visualization of frequencies) as an image.
  - Optionally saves FFT metrics (mean/std dev of log magnitude excluding DC) to a text file.
  - Usage: `python scripts/fft_analyzer.py --input <image_path> --output_img <fft_image_path> [options]`
  - Options:
    - `--input PATH` (Required): Path to the input image.
    - `--output_img PATH` (Required): Path to save the FFT spectrum image.
    - `--output_txt PATH` (Optional): Path to save metrics text file.
    - `--output_dir DIR` (Optional): Directory for output files (defaults to output_img directory or current dir).

## How it Works

1.  **Hashing:** The input string (and its reverse) are hashed using SHA-256, producing 32-byte hashes.
2.  **Parameter Derivation:** Different segments of the hash bytes are interpreted as seeds (normalized to `[0, 1]`) for various parameters:
    - Gradient angle
    - Warp field frequencies, amplitudes, and phases (X and Y)
    - Hill wave frequency, amplitude, and phase
    - Color order permutation
3.  **Image Generation:** An RGBA image is created. For each pixel:
    - Its coordinates are potentially warped based on sine functions derived from warp parameters.
    - A base gradient value (`tBase`) is calculated based on the (potentially warped) coordinates projected onto the gradient angle vector.
    - A hill wave modification (`tWave`) is calculated based on the pixel's _original_ coordinates and hill parameters.
    - The final value `tFinal` is derived by combining `tBase` and `tWave`, then clamped to `[0, 1]`.
    - The pixel color is determined by linearly interpolating between three base colors based on `tFinal`. The order of these three colors is determined by the color order parameter.
4.  **Concurrency & Blending:** Two images are generated concurrently (one for the original string hash, one for the reversed string hash). These two images are then blended pixel-by-pixel (averaging RGB values) in parallel.
5.  **Output:** The final blended image is saved as a PNG file.

## Citation

If you use this work in your research, please cite:

```bibtex
@article{legg2025hashgrad,
  title={HashGrad: Deterministic Visual Parameter Mapping for Reproducible Gradient Synthesis in AI Training},
  author={Legg, Jordan and {Takara.ai}},
  journal={Takara.ai Research},
  year={2025},
  url={https://takara.ai/papers/HashGrad.pdf}
}
```

---

For research inquiries and press, please reach out to research@takara.ai

> 人類を変革する
