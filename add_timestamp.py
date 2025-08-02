import time

# Path to your local metrics file
INPUT_FILE = "sample_data"

# Auto-detect encoding
with open(INPUT_FILE, "rb") as f:
    raw = f.read()

# Check BOM for UTF-16 LE or BE
if raw.startswith(b'\xff\xfe') or raw.startswith(b'\xfe\xff'):
    metrics = raw.decode("utf-16")
else:
    metrics = raw.decode("utf-8")

# Get current Unix timestamp in seconds
timestamp = str(int(time.time()))

output_lines = []

for line in metrics.splitlines():
    if not line or line.startswith("#"):
        # Keep HELP, TYPE, and blank lines unchanged
        output_lines.append(line)
        continue

    parts = line.split()

    if len(parts) == 2:
        # Append timestamp if metric only has name/value
        output_lines.append(f"{parts[0]} {parts[1]} {timestamp}")
    else:
        # Replace any existing timestamp with the current one
        output_lines.append(f"{' '.join(parts[:2])} {timestamp}")

# Print modified metrics
print("\n".join(output_lines))
