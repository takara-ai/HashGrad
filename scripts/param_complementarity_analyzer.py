import hashlib
import struct
import numpy as np
import random
import string
import math
import os

# --- Constants ---
NUM_STRINGS = 10000  # Number of random strings to generate and test
MIN_STR_LEN = 5
MAX_STR_LEN = 50
OUTPUT_DIR = "analysis/complementarity"
OUTPUT_FILE = os.path.join(OUTPUT_DIR, "parameter_correlations.txt")

# Create output directory if it doesn't exist
os.makedirs(OUTPUT_DIR, exist_ok=True)

# --- Helper Functions ---

def reverse_string(s):
    """Reverses a string."""
    return s[::-1]

def generate_random_string(min_len, max_len):
    """Generates a random string of variable length."""
    length = random.randint(min_len, max_len)
    # Use ascii letters, digits, and some punctuation
    characters = string.ascii_letters + string.digits + string.punctuation
    return ''.join(random.choice(characters) for _ in range(length))

def extract_parameter_seeds(hash_bytes):
    """Extracts normalized parameter seeds (0-1 range) from SHA256 hash bytes,
       mirroring the logic in main.go."""
    seeds = {}
    
    # Ensure hash_bytes is 32 bytes long
    if len(hash_bytes) != 32:
        raise ValueError("Hash bytes must be 32 bytes long")

    try:
        # Bytes 0-7 (8 bytes) for Linear Gradient Angle Seed
        # Use struct to unpack as unsigned long long (uint64)
        angle_int = struct.unpack('>Q', hash_bytes[0:8])[0]
        seeds['angleSeed'] = float(angle_int) / float(0xFFFFFFFFFFFFFFFF) # MaxUint64

        # Bytes 8-11 (4 bytes) for Warp X Frequency Seed
        warpfreqx_int = struct.unpack('>I', hash_bytes[8:12])[0] # uint32
        seeds['warpFreqXSeed'] = float(warpfreqx_int) / float(0xFFFFFFFF) # MaxUint32

        # Bytes 12-13 (2 bytes) for Warp X Amplitude Seed
        warpadx_int = struct.unpack('>H', hash_bytes[12:14])[0] # uint16
        seeds['warpAmpXSeed'] = float(warpadx_int) / float(0xFFFF) # MaxUint16

        # Bytes 14-15 (2 bytes) for Warp X Phase Seed
        warpphasex_int = struct.unpack('>H', hash_bytes[14:16])[0] # uint16
        seeds['warpPhaseXSeed'] = float(warpphasex_int) / float(0xFFFF) # MaxUint16

        # Bytes 16-19 (4 bytes) for Warp Y Frequency Seed
        warpfreqy_int = struct.unpack('>I', hash_bytes[16:20])[0] # uint32
        seeds['warpFreqYSeed'] = float(warpfreqy_int) / float(0xFFFFFFFF) # MaxUint32

        # Bytes 20-21 (2 bytes) for Warp Y Amplitude Seed
        warampy_int = struct.unpack('>H', hash_bytes[20:22])[0] # uint16
        seeds['warpAmpYSeed'] = float(warampy_int) / float(0xFFFF) # MaxUint16

        # Bytes 22-23 (2 bytes) for Warp Y Phase Seed
        warpphasey_int = struct.unpack('>H', hash_bytes[22:24])[0] # uint16
        seeds['warpPhaseYSeed'] = float(warpphasey_int) / float(0xFFFF) # MaxUint16

        # Bytes 24-27 (4 bytes) for Hill Wave Frequency Seed
        hillfreq_int = struct.unpack('>I', hash_bytes[24:28])[0] # uint32
        seeds['hillFreqSeed'] = float(hillfreq_int) / float(0xFFFFFFFF) # MaxUint32

        # Bytes 28-29 (2 bytes) for Hill Wave Phase Shift Seed
        hillphase_int = struct.unpack('>H', hash_bytes[28:30])[0] # uint16
        seeds['hillPhaseSeed'] = float(hillphase_int) / float(0xFFFF) # MaxUint16

        # Byte 30 (1 byte) for Color Order Seed (raw index / 6 not needed for correlation)
        seeds['orderIndexSeed'] = float(hash_bytes[30]) / 255.0 # Normalize the byte value

        # Byte 31 (1 byte) for Hill Wave Amplitude Seed
        hillamp_int = hash_bytes[31] # uint8
        seeds['hillAmpSeed'] = float(hillamp_int) / 255.0 # MaxUint8

    except struct.error as e:
        print(f"Error unpacking hash bytes: {e}")
        return None
    except IndexError as e:
        print(f"Error accessing hash bytes (IndexError): {e}")
        return None
        
    return seeds

# --- Main Analysis ---

print(f"Starting Hash Parameter Complementarity Analysis for {NUM_STRINGS} strings...")

# Store lists of seeds for each parameter type (original vs reversed)
param_data = {}
param_names = []

# Generate strings and extract seeds
processed_count = 0
for i in range(NUM_STRINGS):
    s_orig = generate_random_string(MIN_STR_LEN, MAX_STR_LEN)
    s_rev = reverse_string(s_orig)

    # Hash original and reversed strings
    hash_orig = hashlib.sha256(s_orig.encode('utf-8', errors='ignore')).digest()
    hash_rev = hashlib.sha256(s_rev.encode('utf-8', errors='ignore')).digest()

    # Extract seeds
    seeds_orig = extract_parameter_seeds(hash_orig)
    seeds_rev = extract_parameter_seeds(hash_rev)

    if seeds_orig is None or seeds_rev is None:
        print(f"Skipping string {i} due to seed extraction error.")
        continue

    # Initialize storage on first successful extraction
    if not param_data:
        param_names = list(seeds_orig.keys())
        for name in param_names:
            param_data[name] = {'orig': [], 'rev': []}

    # Store the seeds
    for name in param_names:
        param_data[name]['orig'].append(seeds_orig[name])
        param_data[name]['rev'].append(seeds_rev[name])
        
    processed_count += 1
    if (i + 1) % (NUM_STRINGS // 10) == 0:
        print(f"  Processed {i+1}/{NUM_STRINGS} strings...")

if processed_count == 0:
    print("No strings were processed successfully. Exiting.")
    exit()

print(f"Processed {processed_count} strings successfully.")
print("Calculating correlations between original and reversed string hash parameters...")

# Calculate and store correlations
correlations = {}
output_lines = []

output_lines.append(f"Correlation Analysis Results ({processed_count} random strings)")
output_lines.append("="*50)
output_lines.append(f"{'Parameter Seed':<20} | {'Correlation(orig, rev)':<25}")
output_lines.append("-"*50)

for name in param_names:
    try:
        # Ensure lists are numpy arrays for corrcoef
        orig_seeds = np.array(param_data[name]['orig'])
        rev_seeds = np.array(param_data[name]['rev'])
        
        # Calculate Pearson correlation coefficient
        # corrcoef returns a 2x2 matrix: [[corr(orig,orig), corr(orig,rev)],
        #                                [corr(rev,orig), corr(rev,rev)]]
        correlation_matrix = np.corrcoef(orig_seeds, rev_seeds)
        correlation = correlation_matrix[0, 1] # Get the off-diagonal element
        
        # Check for NaN (can happen if variance is zero, though unlikely here)
        if np.isnan(correlation):
            correlation = 0.0 # Treat as no correlation if NaN occurs
            
        correlations[name] = correlation
        print(f"  {name:<20}: {correlation:.6f}")
        output_lines.append(f"{name:<20} | {correlation:<25.6f}")

    except Exception as e:
        print(f"  Error calculating correlation for {name}: {e}")
        correlations[name] = "Error"
        output_lines.append(f"{name:<20} | {'Error calculating':<25}")

output_lines.append("-"*50)
print("-"*50)

# --- Save results to file ---
try:
    with open(OUTPUT_FILE, 'w') as f:
        for line in output_lines:
            f.write(line + '\n')
    print(f"\nCorrelation results saved to: {OUTPUT_FILE}")
except IOError as e:
    print(f"\nError writing results to file {OUTPUT_FILE}: {e}")

print("\nAnalysis complete.") 