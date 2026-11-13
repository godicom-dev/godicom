"""Dev-time parity check: pydicom write-side encapsulation vs godicom.

Produces a JSON of golden outputs for fixed inputs; the Go test
TestEncapsulatedBufferParity re-runs the same inputs through the Go API and
diffs against this file's expectations (ported as literals).
"""
import json
import os
import re
import sys
import tempfile

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))


def load_pydicom():
    """Import pydicom from the submodule, installed or not.

    Same trick as scripts/gen-charset-fixtures.py: a synthetic dist-info on
    sys.path answers the version metadata pydicom asks for at import time.
    """
    src = os.path.join(ROOT, "pydicom", "src")
    version = "0.0.0"
    version_file = os.path.join(src, "pydicom", "_version.py")
    if os.path.isfile(version_file):
        found = re.search(r'__version__\s*=\s*["\']([^"\']+)', open(version_file).read())
        if found:
            version = found.group(1)
    meta_root = tempfile.mkdtemp(prefix="pydicom-meta-")
    dist_info = os.path.join(meta_root, "pydicom-%s.dist-info" % version)
    os.makedirs(dist_info, exist_ok=True)
    with open(os.path.join(dist_info, "METADATA"), "w") as fh:
        fh.write("Metadata-Version: 2.1\nName: pydicom\nVersion: %s\n" % version)
    sys.path.insert(0, src)
    sys.path.insert(0, meta_root)
    import pydicom


load_pydicom()

from io import BytesIO

from pydicom import dcmread
from pydicom.encaps import (
    EncapsulatedBuffer,
    encapsulate_buffer,
    encapsulate_extended_buffer,
    itemize_frame,
    parse_fragments,
    generate_frames,
)

out = {}

# --- synthetic frames: even, odd, empty, mixed -------------------------------
frames = [b"\x00\x01" * 10, b"\xff", b"", b"\x02\x03\x04\x05"]
bframes = [BytesIO(f) for f in frames]

out["frames_lengths"] = [len(f) for f in frames]
out["buffer_lengths"] = EncapsulatedBuffer(bframes).lengths
out["buffer_offsets"] = EncapsulatedBuffer(bframes).offsets
out["buffer_encapsulated_length"] = EncapsulatedBuffer(bframes).encapsulated_length

eb = encapsulate_buffer(bframes, has_bot=True)
out["encapsulate_buffer_bot"] = eb.read(8192).hex()
eb.seek(0)
out["encapsulate_buffer_nobot"] = encapsulate_buffer(bframes, has_bot=False).read(8192).hex()

out["extended_offsets"] = EncapsulatedBuffer(bframes).extended_offsets.hex()
out["extended_lengths"] = EncapsulatedBuffer(bframes).extended_lengths.hex()

pix, eoff, elen = encapsulate_extended_buffer(bframes)
pix.seek(0)
out["encapsulate_extended_buffer"] = (pix.read(8192).hex(), eoff.hex(), elen.hex())

# --- chunked read trace (mirrors pydicom TestRead pattern) --------------------
eb = EncapsulatedBuffer(
    [BytesIO(b"\x00\x01" * 150), BytesIO(b"\x02\x03\x04\x05"), BytesIO(b"\x06\x07\x08" * 111)]
)
trace = []
for size in [6, 13, 294, 12, 12, 108, 8192, 8192]:
    trace.append(eb.read(size).hex())
trace.append(eb.read(100).hex())  # at EOF
out["read_trace"] = trace
out["read_trace_tell"] = eb.tell()

# seek trace: (offset, whence) -> position
eb.seek(0)
seek_trace = []
for off, whence in [(8, 0), (23, 0), (-1, 1), (1, 1), (30, 1), (0, 2), (-7, 2), (-17, 2)]:
    seek_trace.append(eb.seek(off, whence))
out["seek_trace"] = seek_trace

# --- real J2K fixture ---------------------------------------------------------
ds = dcmread(os.path.join(ROOT, "testdata", "dcm", "emri_small_jpeg_2k_lossless.dcm"))
rframes = [BytesIO(f) for f in generate_frames(ds.PixelData, number_of_frames=ds.NumberOfFrames)]
out["fixture_nr_frames"] = len(rframes)

eb2 = encapsulate_buffer(rframes, has_bot=True)
out["fixture_encapsulated_bot"] = eb2.read(8192).hex()
eb2.seek(0)
out["fixture_encapsulated_nobot"] = encapsulate_buffer(rframes, has_bot=False).read(8192).hex()
out["fixture_offsets"] = EncapsulatedBuffer(rframes, use_bot=True).offsets
out["fixture_extended_offsets"] = EncapsulatedBuffer(rframes).extended_offsets.hex()
out["fixture_extended_lengths"] = EncapsulatedBuffer(rframes).extended_lengths.hex()

# --- itemize_frame -------------------------------------------------------------
out["itemize_frame_1"] = [i.hex() for i in itemize_frame(b"\x01\x02" * 5, 1)]
out["itemize_frame_2"] = [i.hex() for i in itemize_frame(b"\x01\x02" * 5, 2)]
out["itemize_frame_3"] = [i.hex() for i in itemize_frame(b"\x01\x02\x03\x04\x05", 3)]

# --- parse_fragments ------------------------------------------------------------
multi = (
    b"\xfe\xff\x00\xe0\x04\x00\x00\x00\x01\x00\x00\x00"
    b"\xfe\xff\x00\xe0\x06\x00\x00\x00\x01\x02\x03\x04\x05\x06"
    b"\xfe\xff\xdd\xe0\x00\x00\x00\x00"
)
out["parse_fragments_multi"] = parse_fragments(multi)
out["parse_fragments_short"] = None
try:
    parse_fragments(b"\xfe\xff\x00\xe0\x04\x00\x00")
except ValueError as e:
    out["parse_fragments_short"] = str(e)
out["parse_fragments_undef"] = None
try:
    parse_fragments(b"\xfe\xff\x00\xe0\xff\xff\xff\xff\x00\x00\x00\x01")
except ValueError as e:
    out["parse_fragments_undef"] = str(e)
out["parse_fragments_badtag"] = None
try:
    parse_fragments(
        b"\xfe\xff\x00\xe0\x04\x00\x00\x00\x01\x00\x00\x00\x10\x00\x10\x00\x00\x00\x00\x00"
    )
except ValueError as e:
    out["parse_fragments_badtag"] = str(e)

print(json.dumps(out, indent=1))
