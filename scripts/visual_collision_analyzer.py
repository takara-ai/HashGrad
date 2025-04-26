import os
import sys
import subprocess
import time
import random
import string
import argparse
from collections import defaultdict
import imagehash # pip install ImageHash
import uuid      # <-- Import uuid
import tempfile  # <-- Import tempfile

def generate_random_string(length=16):
    """Generates a random alphanumeric string."""
    characters = string.ascii_letters + string.digits
    return ''.join(random.choice(characters) for i in range(length))

def calculate_hashes(image_path):
    """Calculates perceptual hashes (phash, dhash) for an image."""
    try:
        from PIL import Image
        img = Image.open(image_path)
        phash_val = str(imagehash.phash(img))
        dhash_val = str(imagehash.dhash(img))
        return phash_val, dhash_val
    except Exception as e:
        print(f"Error processing image {image_path} for hashing: {e}", file=sys.stderr)
        return None, None

def hamming_distance(s1, s2):
    """Calculates the Hamming distance between two hex strings."""
    if len(s1) != len(s2):
        raise ValueError("Strings must be of the same length")
    # Convert hex strings to integers and then compare bits
    # Alternatively, compare hex digits directly if format is consistent
    # This assumes hex strings represent binary data
    # A simpler approach for typical image hashes:
    return sum(c1 != c2 for c1, c2 in zip(s1, s2))

if __name__ == "__main__":
    parser = argparse.ArgumentParser(description='Analyze visual collisions using perceptual hashing.')
    parser.add_argument('--num_samples', type=int, default=1000, help='Number of random strings to test.')
    parser.add_argument('--go_executable', default='./txt-gradient', help='Path to the compiled Go executable.')
    parser.add_argument('--summary_file', default='analysis/collisions/visual_collision_summary.txt', help='Path to save the summary results.')
    parser.add_argument('--max_near_distance', type=int, default=4, help='Maximum Hamming distance to consider as a near collision.')
    parser.add_argument('--temp_dir', default=tempfile.gettempdir(), help='Directory to store temporary image files.')

    args = parser.parse_args()

    start_time = time.time()
    results = {}
    failed_strings = []
    phash_map = defaultdict(list) # Map phash -> list of input strings
    dhash_map = defaultdict(list) # Map dhash -> list of input strings
    unique_phashes = set()
    unique_dhashes = set()

    print(f"Starting Visual Collision Analysis with {args.num_samples} samples...")
    print(f"Using Go executable: {args.go_executable}")
    print(f"Using temp directory: {args.temp_dir}")

    # Ensure output and temp directories exist
    summary_dir = os.path.dirname(args.summary_file)
    if summary_dir:
        os.makedirs(summary_dir, exist_ok=True)
    if args.temp_dir:
        os.makedirs(args.temp_dir, exist_ok=True)

    for i in range(args.num_samples):
        input_string = generate_random_string()
        unique_filename = os.path.join(args.temp_dir, f"gradient_{uuid.uuid4()}.png")
        process = None # Initialize process variable
        
        # Run the Go program with the unique output filename
        try:
            # Construct command with --output flag
            command = [args.go_executable, "--output", unique_filename, input_string]
            # print(f"Running command: {' '.join(command)}") # Debugging
            process = subprocess.run(command, capture_output=True, text=True, check=True, timeout=60) # Increased timeout slightly
            # Optional: check process.stdout/stderr if needed
        except subprocess.CalledProcessError as e:
            print(f"Error running Go program for string '{input_string}' -> {unique_filename}: {e}", file=sys.stderr)
            print(f"Stderr: {e.stderr}", file=sys.stderr)
            failed_strings.append(input_string)
            # Clean up potentially partially created file
            if os.path.exists(unique_filename):
                 try: os.remove(unique_filename)
                 except OSError: pass
            continue
        except subprocess.TimeoutExpired:
            print(f"Timeout running Go program for string '{input_string}' -> {unique_filename}", file=sys.stderr)
            failed_strings.append(input_string)
            # Clean up potentially partially created file
            if os.path.exists(unique_filename):
                 try: os.remove(unique_filename)
                 except OSError: pass
            continue
        except Exception as e: # Catch other potential errors like file not found for executable
             print(f"General error running Go program for string '{input_string}': {e}", file=sys.stderr)
             failed_strings.append(input_string)
             continue
            
        # Check if output file was created
        if not os.path.exists(unique_filename):
            print(f"Go program did not create output file '{unique_filename}' for string '{input_string}'", file=sys.stderr)
            # Optionally print stdout/stderr from process if available
            if process:
                 print(f"Stdout: {process.stdout}", file=sys.stderr)
                 print(f"Stderr: {process.stderr}", file=sys.stderr)
            failed_strings.append(input_string)
            continue
            
        # Calculate hashes using the unique filename
        phash_val, dhash_val = calculate_hashes(unique_filename)
        
        if phash_val is not None and dhash_val is not None:
            results[input_string] = {"phash": phash_val, "dhash": dhash_val}
            phash_map[phash_val].append(input_string)
            dhash_map[dhash_val].append(input_string)
            unique_phashes.add(phash_val)
            unique_dhashes.add(dhash_val)
        else:
            failed_strings.append(input_string)
            
        # Clean up the temporary file
        if os.path.exists(unique_filename):
            try:
                os.remove(unique_filename)
            except OSError as e:
                print(f"Warning: Could not remove temporary file {unique_filename}: {e}", file=sys.stderr)
            
        if (i + 1) % (args.num_samples // 20) == 0 or (i + 1) == args.num_samples: # Report more often
            print(f"Processed {i + 1}/{args.num_samples} strings...")

    end_time = time.time()
    duration = end_time - start_time

    # --- Analyze Results --- 
    num_processed = len(results)
    num_failed = len(failed_strings)
    
    # PHash Analysis
    exact_phash_collisions = {h: strings for h, strings in phash_map.items() if len(strings) > 1}
    num_exact_phash_collisions = sum(len(strings) for strings in exact_phash_collisions.values()) - len(exact_phash_collisions)
    num_phash_colliding_groups = len(exact_phash_collisions)
    unique_phash_list = list(unique_phashes)
    near_phash_collisions_count = 0
    near_phash_pairs = []
    for i in range(len(unique_phash_list)):
        for j in range(i + 1, len(unique_phash_list)):
            dist = hamming_distance(unique_phash_list[i], unique_phash_list[j])
            if dist <= args.max_near_distance:
                near_phash_collisions_count += 1
                near_phash_pairs.append((unique_phash_list[i], unique_phash_list[j], dist))
                
    # DHash Analysis
    exact_dhash_collisions = {h: strings for h, strings in dhash_map.items() if len(strings) > 1}
    num_exact_dhash_collisions = sum(len(strings) for strings in exact_dhash_collisions.values()) - len(exact_dhash_collisions)
    num_dhash_colliding_groups = len(exact_dhash_collisions)
    unique_dhash_list = list(unique_dhashes)
    near_dhash_collisions_count = 0
    near_dhash_pairs = []
    for i in range(len(unique_dhash_list)):
        for j in range(i + 1, len(unique_dhash_list)):
            dist = hamming_distance(unique_dhash_list[i], unique_dhash_list[j])
            if dist <= args.max_near_distance:
                near_dhash_collisions_count += 1
                near_dhash_pairs.append((unique_dhash_list[i], unique_dhash_list[j], dist))

    # --- Write Summary --- 
    print(f"Writing summary to {args.summary_file}...")
    with open(args.summary_file, 'w') as f:
        f.write(f"Visual Collision Analysis Summary ({time.ctime()})\n")
        f.write("="*40 + "\n")
        f.write("Parameters:\n")
        f.write(f"  Number of strings tested: {args.num_samples}\n")
        f.write(f"  Number of strings processed successfully: {num_processed}\n")
        f.write(f"  Number of strings failed: {num_failed}\n")
        f.write(f"  Go executable: {args.go_executable}\n")
        f.write(f"  Near collision threshold (Hamming): <= {args.max_near_distance}\n")
        f.write(f"  Analysis duration: {duration:.2f} seconds\n\n")
        
        f.write("--- PHash Results ---\n")
        f.write(f"  Unique phashes generated: {len(unique_phashes)}\n")
        f.write(f"  Strings in exact collisions: {num_exact_phash_collisions}")
        f.write(f" ({num_phash_colliding_groups} colliding groups)\n")
        exact_phash_rate = (num_exact_phash_collisions / num_processed * 100) if num_processed > 0 else 0
        f.write(f"  Exact PHash Collision Rate (strings involved): {exact_phash_rate:.4f}%\n")
        f.write(f"  Near collision pairs (dist <= {args.max_near_distance}): {near_phash_collisions_count}\n")
        near_phash_pair_rate = (near_phash_collisions_count / (len(unique_phashes) * (len(unique_phashes)-1) / 2) * 100) if len(unique_phashes) > 1 else 0
        f.write(f"  Near PHash Collision Rate (pairs): {near_phash_pair_rate:.4f}%\n\n")

        f.write("--- DHash Results ---\n")
        f.write(f"  Unique dhashes generated: {len(unique_dhashes)}\n")
        f.write(f"  Strings in exact collisions: {num_exact_dhash_collisions}")
        f.write(f" ({num_dhash_colliding_groups} colliding groups)\n")
        exact_dhash_rate = (num_exact_dhash_collisions / num_processed * 100) if num_processed > 0 else 0
        f.write(f"  Exact DHash Collision Rate (strings involved): {exact_dhash_rate:.4f}%\n")
        f.write(f"  Near collision pairs (dist <= {args.max_near_distance}): {near_dhash_collisions_count}\n")
        near_dhash_pair_rate = (near_dhash_collisions_count / (len(unique_dhashes) * (len(unique_dhashes)-1) / 2) * 100) if len(unique_dhashes) > 1 else 0
        f.write(f"  Near DHash Collision Rate (pairs): {near_dhash_pair_rate:.4f}%\n\n")
        
        if exact_phash_collisions:
            f.write("Exact PHash Colliding Groups:\n")
            for h, strings in exact_phash_collisions.items():
                f.write(f"  Hash {h}: {strings}\n")
            f.write("\n")
        if near_phash_pairs:
             f.write(f"Near PHash Colliding Pairs (Hash1, Hash2, Distance <= {args.max_near_distance}):\n")
             for h1, h2, dist in near_phash_pairs[:20]: # Limit output
                 f.write(f"  ({h1}, {h2}, {dist})\n")
             if len(near_phash_pairs) > 20:
                 f.write("  ... (further pairs omitted)\n")
             f.write("\n")

        # Add similar detailed output for dhash if needed

    print(f"Analysis complete. Summary saved to {args.summary_file}") 