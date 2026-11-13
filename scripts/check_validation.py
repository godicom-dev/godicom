"""Dev-time parity check: pydicom's value validation modes vs godicom.

pydicom's read leaves every element raw: validation happens when a value is
first converted, so the mode is exercised per element access. This script
therefore reads one bad value per dataset and accesses exactly that element,
under each of the three reading_validation_mode settings, and prints a JSON of
what happens. The Go test TestValueValidationParity ports the same
expectations as literals; godicom converts at read time, so its warn and
raise fire during the read itself, and the values and rejections must agree.
"""
import io
import json
import os
import re
import struct
import sys
import tempfile
import warnings

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))


def load_pydicom():
    """Import pydicom from the submodule, installed or not."""
    src = os.path.join(ROOT, "pydicom", "src")
    version = "0.0.0"
    version_file = os.path.join(src, "pydicom", "_version.py")
    if os.path.isfile(version_file):
        found = re.search(r'__version__\s*=\s*["\']([^"\']+)', open(version_file).read())
        if found:
            version = found.group(1)
    meta_root = tempfile.mkdtemp(prefix="pydicom-meta-")
    dist_info = os.path.join(meta_root, "pydicom-%s.dist-info" % version)
    os.makedirs(dist_info)
    with open(os.path.join(dist_info, "METADATA"), "w") as f:
        f.write("Metadata-Version: 2.1\nName: pydicom\nVersion: %s\n" % version)
    sys.path.insert(0, meta_root)
    sys.path.insert(0, src)


load_pydicom()

import pydicom  # noqa: E402
from pydicom import config  # noqa: E402
from pydicom.tag import Tag  # noqa: E402


def explicit_elem(tag, vr, value, pad=True):
    """One explicit VR little endian element."""
    if pad and len(value) % 2:
        value += b" "
    out = struct.pack("<HH", tag.group, tag.element)
    out += vr.encode("ascii")
    if vr in ("OB", "OD", "OF", "OL", "OV", "OW", "SQ", "SV", "UC", "UN", "UR", "UT", "UV"):
        out += struct.pack("<HI", 0, len(value))
    else:
        out += struct.pack("<H", len(value))
    return out + value


def read_and_access(data, tag):
    ds = pydicom.filereader.read_dataset(io.BytesIO(data), False, True, len(data))
    return str(ds[tag].value)


# One bad value per dataset, and one legal neighbour per VR to pin that the
# same mode raises nothing when every value is legal.
cases = {
    "cs_bad": (Tag(0x0018, 0x0015), "CS", b"knee"),
    "cs_good": (Tag(0x0018, 0x0015), "CS", b"KNEE"),
    "ui_bad": (Tag(0x0008, 0x0018), "UI", b"01.2.3"),
    "ui_good": (Tag(0x0008, 0x0018), "UI", b"1.2.3"),
    "lo_long": (Tag(0x0008, 0x1030), "LO", b"x" * 65),
    "lo_good": (Tag(0x0008, 0x1030), "LO", b"x" * 64),
}

out = {}
for name, (tag, vr, value) in cases.items():
    data = explicit_elem(tag, vr, value)
    entry = {"input": data.hex()}
    for mode_name, mode in [
        ("warn", config.WARN),
        ("raise", config.RAISE),
        ("ignore", config.IGNORE),
    ]:
        config.settings.reading_validation_mode = mode
        with warnings.catch_warnings(record=True) as caught:
            warnings.simplefilter("always")
            try:
                v = read_and_access(data, tag)
                entry[mode_name] = {"error": None, "value": v, "warnings": len(caught)}
            except Exception as exc:
                entry[mode_name] = {
                    "error": "%s: %s" % (type(exc).__name__, exc),
                    "value": None,
                    "warnings": 0,
                }
    out[name] = entry

print(json.dumps(out, indent=1, sort_keys=True))
