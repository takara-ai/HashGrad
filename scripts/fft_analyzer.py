import numpy as np
import cv2 # OpenCV for image loading
import os
import sys
import json
import argparse

def create_hann_window_2d(rows, cols):
    """Creates a 2D Hann window."""
    hann_rows = np.hanning(rows)
    hann_cols = np.hanning(cols)
    # Use outer product to create 2D window
    window_2d = np.outer(hann_rows, hann_cols)
    return window_2d

def analyze_fft(image_path, output_image_path, output_txt_path, params={}):
    """Performs windowed FFT analysis on an image and saves the raw
       log magnitude spectrum as a grayscale image file.
       Also saves the original parameters and calculated FFT metrics to a text file.

       Windowing (Hann) is applied before FFT to reduce edge artifacts.
    """
    # Load image in grayscale
    img_gray = cv2.imread(image_path, cv2.IMREAD_GRAYSCALE)
    if img_gray is None:
        print(f"  Error loading image: {image_path}", file=sys.stderr)
        return False

    # --- Apply 2D Hann Window ---
    try:
        rows, cols = img_gray.shape
        window = create_hann_window_2d(rows, cols)
        windowed_img = img_gray * window
    except Exception as e:
        print(f"  Error applying window function for {image_path}: {e}", file=sys.stderr)
        return False

    # --- Perform 2D FFT on windowed image ---
    try:
        f_transform = np.fft.fft2(windowed_img)
        f_shift = np.fft.fftshift(f_transform)
        # Calculate log magnitude spectrum (+1 to avoid log(0))
        magnitude_spectrum = np.log(np.abs(f_shift) + 1)
    except Exception as e:
        print(f"  Error during FFT processing for {image_path}: {e}", file=sys.stderr)
        return False

    # --- Calculate FFT Metrics (excluding DC component) ---
    fft_mean = None
    fft_stddev = None
    try:
        center_y, center_x = rows // 2, cols // 2
        # Create a mask to exclude the center pixel (DC component)
        mask = np.ones(magnitude_spectrum.shape, dtype=bool)
        mask[center_y, center_x] = False
        # Calculate mean and std dev on non-DC components
        fft_mean = np.mean(magnitude_spectrum[mask])
        fft_stddev = np.std(magnitude_spectrum[mask])
        # Add metrics to params dictionary if provided
        if params:
             params['fft_mean_log_magnitude_no_dc'] = fft_mean
             params['fft_stddev_log_magnitude_no_dc'] = fft_stddev
        # print(f"  Calculated FFT Metrics: Mean={fft_mean:.4f}, StdDev={fft_stddev:.4f}") # Less verbose for video
    except Exception as e:
        print(f"  Error calculating FFT metrics for {image_path}: {e}", file=sys.stderr)
        # Continue even if metrics calculation fails
        if params:
            params['fft_mean_log_magnitude_no_dc'] = 'Error'
            params['fft_stddev_log_magnitude_no_dc'] = 'Error'

    # --- Normalize and save the raw magnitude spectrum image ---
    try:
        # Normalize the spectrum to 0-255 range for saving as image
        min_val, max_val = np.min(magnitude_spectrum), np.max(magnitude_spectrum)
        if max_val > min_val:
            normalized_spectrum = cv2.normalize(magnitude_spectrum, None, 0, 255, cv2.NORM_MINMAX)
        else:
            normalized_spectrum = np.zeros_like(magnitude_spectrum) # Avoid division by zero if flat
        
        # Convert to 8-bit unsigned integer
        spectrum_image = normalized_spectrum.astype(np.uint8)

        # Save the raw spectrum image using OpenCV
        cv2.imwrite(output_image_path, spectrum_image)
        # print(f"  Successfully saved raw FFT spectrum image to: {output_image_path}") # Less verbose

    except Exception as e:
        print(f"  Error normalizing or saving spectrum image for {image_path}: {e}", file=sys.stderr)
        # Continue to try saving the parameters even if image saving fails

    # --- Save the parameters and metrics to a text file (Optional) ---
    if output_txt_path and params:
        try:
            # Format parameters nicely using json indent
            param_string = json.dumps(params, indent=4)
            with open(output_txt_path, 'w') as f:
                f.write(f"Parameters for original image: {image_path}\n")
                f.write(f"Corresponding raw FFT spectrum image: {os.path.basename(output_image_path)}\n\n") # Note: raw spectrum
                f.write(param_string)
            # print(f"  Successfully saved parameters and metrics to: {output_txt_path}") # Less verbose
        except Exception as e:
            print(f"  Error saving parameters for {image_path}: {e}", file=sys.stderr)
            # Don't necessarily mark overall as failure if only text saving fails
            
    return True # Indicate overall success if image was processed


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description='Perform windowed FFT analysis on a single image.')
    parser.add_argument('--input', required=True, help='Path to the input image file.')
    parser.add_argument('--output_img', required=True, help='Path to save the output FFT spectrum image (e.g., fft_frames/fft_frame_0001.png).')
    parser.add_argument('--output_txt', required=False, help='Optional: Path to save parameters and metrics text file.')
    # Add output_dir argument
    parser.add_argument('--output_dir', required=False, help='Directory to save the output files (img/txt). Defaults to current dir if not provided.')

    args = parser.parse_args()

    # Determine output directory
    if args.output_dir:
        output_dir = args.output_dir
        os.makedirs(output_dir, exist_ok=True)
    else:
        # Default to the directory of the output image if output_dir not specified
        output_dir = os.path.dirname(args.output_img)
        if not output_dir:
             output_dir = '.' # Use current dir if path has no directory
        os.makedirs(output_dir, exist_ok=True)
        
    # Construct full output paths
    output_image_path = os.path.join(output_dir, os.path.basename(args.output_img))
    output_txt_path = None
    if args.output_txt:
        output_txt_path = os.path.join(output_dir, os.path.basename(args.output_txt))
        
    # Basic params for context, can be expanded later if needed
    params = {"description": f"FFT Analysis of {os.path.basename(args.input)}"}

    # print(f"Starting Windowed FFT Analysis for: {args.input}") # Less verbose
    # print(f"Output spectrum image: {output_image_path}")
    # if output_txt_path:
    #      print(f"Output metrics file: {output_txt_path}")
         
    if analyze_fft(args.input, output_image_path, output_txt_path, params):
        # print(f"Analysis successful.")
        sys.exit(0) # Success exit code
    else:
        print(f"Analysis failed for {args.input}", file=sys.stderr)
        sys.exit(1) # Failure exit code

# --- Original code processing the list is removed --- 