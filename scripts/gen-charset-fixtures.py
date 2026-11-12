#!/usr/bin/env python3
"""Generate the Chinese PN fixtures in testdata/charset/.

Why pydicom and not godicom: a fixture written by godicom and asserted by
godicom only proves the encoder and the decoder agree with each other. The
bytes have to come from the reference implementation, so these four files are
written by pydicom and the Go tests assert what it says they contain.

Covered here, per issue #88:

  chrGB18030Rare.dcm     a four-byte GB18030 character (U+20000) in a name
  chrGB18030Three.dcm    all three PN component groups under GB18030
  chrGB18030Multi.dcm    a multi-valued PN, where '\\' separates the values
  chrGB18030Invalid.dcm  a truncated four-byte sequence and a byte no code
                         point starts with, written as raw bytes

The last one is the only file pydicom will not produce through the normal
writer: a PN value is encoded on the way out, so the invalid bytes are put in
as a RawDataElement and never touch the encoder.

Usage: scripts/gen-charset-fixtures.py [--check]

  --check   write to a temporary directory and compare against the committed
            files instead of overwriting them (what CI would do if these files
            ever needed to be proven reproducible)
"""

import os
import re
import sys
import tempfile

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
OUT = os.path.join(ROOT, "testdata", "charset")


def load_pydicom():
    """Import pydicom from the submodule, installed or not.

    pydicom/ here is a git submodule carrying the source but no installed
    distribution metadata, and pydicom asks importlib.metadata for its own
    version at import time. A synthetic dist-info on sys.path answers that
    without touching the submodule or requiring a pip install.
    """
    src = os.path.join(ROOT, "pydicom", "src")
    if os.path.isdir(src):
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

    return pydicom


CASES = [
    # Four-byte GB18030: U+20000 is outside the BMP, so it costs four bytes per
    # character and sits in the part of the encoding a BMP-only test never
    # reaches. U+9F98 (龘, three bytes) is mixed in so one name exercises both
    # widths.
    ("chrGB18030Rare.dcm", "GB18030", "\U00020000^\u5d0e=\u9f98^\u5d0e", "1.2.826.0.1.3680043.10.999.1"),
    # alphabetic=ideographic=phonetic: the '=' is a component delimiter, not a
    # value delimiter, and every group here is non-empty.
    ("chrGB18030Three.dcm", "GB18030", "Wang^XiaoDong=\u738b^\u5c0f\u4e1c=Wang^XiaoDong", "1.2.826.0.1.3680043.10.999.2"),
    # Two values separated by '\'; each carries an ideographic component.
    ("chrGB18030Multi.dcm", "GB18030", "\u738b^\u5c0f\u4e1c\\\u674e^\u660e", "1.2.826.0.1.3680043.10.999.3"),
]

# Suffixes carry a fixed instance UID rather than a generated one: the fixture
# has to be byte-identical across runs, or --check can never pass and a
# regenerated file shows up as a diff for no reason.
INVALID = "chrGB18030Invalid.dcm"
INVALID_UID = "1.2.826.0.1.3680043.10.999.4"
# 0x81 0x30 0x81 starts a four-byte sequence and stops after three bytes;
# 0xFF cannot begin any GB18030 code unit. Together they are invalid in two
# different ways, so a decoder that only checks the lead byte is not enough.
INVALID_BYTES = b"\x81\x30\x81\xff\x81\x30\x81"


def build(pydicom, path, charset, name, instance):
    ds = pydicom.dataset.FileDataset(
        path,
        {},
        file_meta=pydicom.dataset.FileMetaDataset(),
        preamble=b"\0" * 128,
    )
    uid = pydicom.uid
    ds.file_meta.MediaStorageSOPClassUID = uid.CTImageStorage
    ds.file_meta.MediaStorageSOPInstanceUID = instance
    ds.file_meta.TransferSyntaxUID = uid.ExplicitVRLittleEndian
    ds.SOPClassUID = uid.CTImageStorage
    ds.SOPInstanceUID = instance
    ds.SpecificCharacterSet = charset
    ds.PatientName = name
    pydicom.dcmwrite(path, ds, enforce_file_format=True)
    return ds


def build_invalid(pydicom, path):
    """Write an undecodable PN value the writer cannot repair on the way out.

    A RawDataElement is not enough: dcmwrite re-encodes a PN value from its
    decoded form, so the bytes it stores are the GB18030 encoding of U+FFFD --
    perfectly legal. The file is written normally first and then the encoded
    name is replaced in place, which keeps every length field correct because
    the replacement has the same number of bytes.
    """
    good_name = "\u738b^\u5c0f\u4e1c"  # 王^小东, seven bytes in GB18030
    ds = build(pydicom, path, "GB18030", good_name, INVALID_UID)
    good = good_name.encode("gb18030")
    if len(good) != len(INVALID_BYTES):
        raise SystemExit("replacement must be the same length as %r" % good)
    data = open(path, "rb").read()
    if data.count(good) != 1:
        raise SystemExit("expected exactly one %r in %s, found %d"
                         % (good, path, data.count(good)))
    with open(path, "wb") as fh:
        fh.write(data.replace(good, INVALID_BYTES))
    return ds


def main():
    check = "--check" in sys.argv[1:]
    target = tempfile.mkdtemp(prefix="godicom-charset-") if check else OUT
    os.makedirs(target, exist_ok=True)

    pydicom = load_pydicom()
    for name, charset, value, instance in CASES:
        path = os.path.join(target, name)
        build(pydicom, path, charset, value, instance)
        print("%-24s %-8s %r" % (name, charset, value))
    path = os.path.join(target, INVALID)
    build_invalid(pydicom, path)
    print("%-24s %-8s raw %r" % (INVALID, "GB18030", INVALID_BYTES))

    if not check:
        return 0

    status = 0
    for name in [c[0] for c in CASES] + [INVALID]:
        new = open(os.path.join(target, name), "rb").read()
        committed = os.path.join(OUT, name)
        old = open(committed, "rb").read() if os.path.isfile(committed) else b""
        if new != old:
            print("differs from the committed copy: %s" % name, file=sys.stderr)
            status = 1
        else:
            print("same: %s" % name)
    return status


if __name__ == "__main__":
    sys.exit(main())
