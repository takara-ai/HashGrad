import numpy as np
import cv2
import os
import random

# --- Constants ---
IMG_WIDTH = 800
IMG_HEIGHT = 600
OUTPUT_DIR = "tests/baselines"
OUTPUT_FILENAME = "random_noise.png"
OUTPUT_PATH = os.path.join(OUTPUT_DIR, OUTPUT_FILENAME)

# Create output directory if it doesn't exist
os.makedirs(OUTPUT_DIR, exist_ok=True)

# --- Generate Random Noise Image ---
print(f"Generating random grayscale noise image ({IMG_WIDTH}x{IMG_HEIGHT})...")

# Create an array of random unsigned 8-bit integers (0-255)
noise_array = np.random.randint(0, 256, size=(IMG_HEIGHT, IMG_WIDTH), dtype=np.uint8)

# --- Save Image ---
try:
    success = cv2.imwrite(OUTPUT_PATH, noise_array)
    if success:
        print(f"Successfully saved random noise image to: {OUTPUT_PATH}")
    else:
        print(f"Error: Failed to save image to {OUTPUT_PATH} (cv2.imwrite returned False)")
except Exception as e:
    print(f"Error saving image to {OUTPUT_PATH}: {e}") 