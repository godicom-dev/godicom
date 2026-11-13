"""Dev-time parity check: pydicom's raw element hooks vs godicom.

Builds three small datasets as raw bytes, reads them through pydicom with the
hooks module's callbacks registered, and prints a JSON of the converted
values. The Go test TestHooksParity ports the same expectations as literals.

  A. fix_separator -- a DS and an IS using ":" as the multi-value separator,
     through raw_element_value_fix_separator.
  B. retry -- a DS element whose bytes are binary US data, through
     raw_element_value_retry with {"DS": ("US", "SS")}.
  C. resolve VR -- an implicit-VR private element the dictionary cannot name,
     through a raw_element_vr callback that delegates to the default and then
     forces US for that one tag.
"""
import io
import json
import os
import re
import struct
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
    os.makedirs(dist_info)
    with open(os.path.join(dist_info, "METADATA"), "w") as f:
        f.write("Metadata-Version: 2.1\nName: pydicom\nVersion: %s\n" % version)
    sys.path.insert(0, meta_root)
    sys.path.insert(0, src)


load_pydicom()

import pydicom  # noqa: E402
from pydicom import config  # noqa: E402
from pydicom.datadict import dictionary_VR, private_dictionary_VR  # noqa: E402
from pydicom.dataset import Dataset  # noqa: E402
from pydicom.hooks import (  # noqa: E402
    hooks,
    raw_element_value,
    raw_element_value_fix_separator,
    raw_element_value_retry,
    raw_element_vr,
)
from pydicom.tag import Tag  # noqa: E402
from pydicom.valuerep import VR  # noqa: E402


def explicit_elem(tag, vr, value, pad=True):
    """One explicit VR little endian element, with the header form pydicom's
    EXPLICIT_VR_LENGTH_32 decides: most VRs take a 16-bit length, the dozen
    long-form VRs take two reserved bytes and a 32-bit length. pad=False writes
    the odd length a broken file declares, which is the case hooks exist for."""
    if pad and len(value) % 2:
        value += b" "
    out = struct.pack("<HH", tag.group, tag.element)
    out += vr.encode("ascii")
    if vr in ("OB", "OD", "OF", "OL", "OV", "OW", "SQ", "SV", "UC", "UN", "UR", "UT", "UV"):
        out += struct.pack("<HI", 0, len(value))
    else:
        out += struct.pack("<H", len(value))
    return out + value


def implicit_elem(tag, value):
    """One implicit VR little endian element."""
    return struct.pack("<HHI", tag.group, tag.element, len(value)) + value


def read_dataset(data, implicit=False):
    return pydicom.filereader.read_dataset(
        io.BytesIO(data), implicit, True, len(data)
    )


def value_repr(v):
    """Values as JSON: recursively, with bytes as hex."""
    if isinstance(v, bytes):
        return {"bytes": v.hex()}
    if isinstance(v, (list, pydicom.multival.MultiValue)):
        return [value_repr(x) for x in v]
    if isinstance(v, (pydicom.valuerep.IS, pydicom.valuerep.DSfloat, pydicom.valuerep.DSdecimal)):
        return {"str": str(v), "original": getattr(v, "original_string", None)}
    if isinstance(v, (int, float, str)):
        return {"str": str(v)}
    return {"repr": repr(v)}


out = {}

# -- A. fix_separator ---------------------------------------------------------
sep_data = (
    explicit_elem(Tag(0x0010, 0x1030), "DS", b"70:5")
    + explicit_elem(Tag(0x0010, 0x1010), "IS", b"3:0")
)
out["fix_separator_input"] = sep_data.hex()

config.settings.reading_validation_mode = config.IGNORE
hooks.register_callback("raw_element_value", raw_element_value_fix_separator)
hooks.register_kwargs(
    "raw_element_kwargs", {"target_VRs": ("DS", "IS"), "separator": b":"}
)
ds = read_dataset(sep_data)
out["fix_separator"] = {
    str(Tag(0x0010, 0x1030)): value_repr(ds[0x00101030].value),
    str(Tag(0x0010, 0x1010)): value_repr(ds[0x00101010].value),
}
out["fix_separator_vrs"] = {
    "weight": str(ds[0x00101030].VR),
    "age": str(ds[0x00101010].VR),
}

# Without the hook, pydicom cannot convert either element: the ":" is not a
# decimal digit or an integer character. Record what the read produces -- an
# error or a fallback -- so the Go test can pin its own answer against it.
hooks.register_callback("raw_element_value", raw_element_value)
try:
    ds_plain = read_dataset(sep_data)
    out["fix_separator_unhooked"] = {
        "weight": value_repr(ds_plain[0x00101030].value),
        "age": value_repr(ds_plain[0x00101010].value),
    }
except Exception as exc:
    out["fix_separator_unhooked"] = {"error": "%s: %s" % (type(exc).__name__, exc)}
hooks.raw_element_kwargs = {}

# -- B. retry -----------------------------------------------------------------
# A US element with five bytes: the numeric converters raise BytesLengthException
# on a byte count that is not a multiple of the value size, so the default
# conversion raises and the retry kicks in. (A DS whose text is garbage does not
# raise under reading_validation_mode IGNORE -- DSfloat keeps the string -- so
# pydicom's documented DS -> US/SS example never actually retries there.)
retry_data = explicit_elem(Tag(0x0028, 0x0100), "US", b"\x39\x00\x39\x00\x39", pad=False)
out["retry_input"] = retry_data.hex()

hooks.register_callback("raw_element_value", raw_element_value_retry)
hooks.register_kwargs("raw_element_kwargs", {"target_VRs": {"US": ("OB", "SS")}})
ds = read_dataset(retry_data)
out["retry"] = {
    "value": value_repr(ds[0x00280100].value),
    "vr": str(ds[0x00280100].VR),
}
hooks.raw_element_kwargs = {}

# -- C. resolve VR ------------------------------------------------------------
# An implicit-VR private element (0009,1001) with two bytes of US data. No
# dictionary entry and no private creator for (0009,0010) in this dataset, so
# the default resolution answers UN.
vr_data = implicit_elem(Tag(0x0009, 0x1001), b"\x39\x00")
out["resolve_vr_input"] = vr_data.hex()


def force_private_us(raw, data, **kwargs):
    raw_element_vr(raw, data, **kwargs)
    if raw.tag == Tag(0x0009, 0x1001):
        data["VR"] = VR.US


hooks.register_callback("raw_element_vr", force_private_us)
ds = read_dataset(vr_data, implicit=True)
out["resolve_vr_forced"] = {
    "value": value_repr(ds[0x00091001].value),
    "vr": str(ds[0x00091001].VR),
}

# And the default, which answers UN and keeps the bytes.
hooks.register_callback("raw_element_vr", raw_element_vr)
ds = read_dataset(vr_data, implicit=True)
out["resolve_vr_default"] = {
    "value": value_repr(ds[0x00091001].value),
    "vr": str(ds[0x00091001].VR),
}

print(json.dumps(out, indent=1, sort_keys=True))
