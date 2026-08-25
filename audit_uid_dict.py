"""Audit uid/dictionary_generated.go against the DICOM standard's UID registry.

The generated dictionary is a mirror of pydicom's _uid_dict.py plus the handful of
UIDs in generate_uid_dict.py's STANDARD_ADDITIONS. Neither source is the standard,
so this script compares the result against PS3.6 directly: Table A-1 (the UID
registry) and Table A-2 (well-known frames of reference, which pydicom folds into
the same dictionary).

    python audit_uid_dict.py                 # fetch part06.xml, then audit
    python audit_uid_dict.py part06.xml      # audit an already-downloaded copy

Exits non-zero if the dictionary and the standard disagree, and prints every
disagreement. Run it after bumping the pydicom submodule, and when a new edition
of the standard is published.

This is deliberately not part of CI. It depends on the network and on a document
that changes without reference to this repository, so wiring it into the gate
would turn an unrelated DICOM release into a red build on somebody's pull request.
The Go-side guard for the entries that have no pydicom backing is
TestUIDsAheadOfPydicom in uid/uid_test.go.
"""
import os
import re
import sys
import urllib.request
import xml.etree.ElementTree as ET

PART06_URL = (
    "https://dicom.nema.org/medical/dicom/current/source/docbook/part06/part06.xml"
)
DOCBOOK = "http://docbook.org/ns/docbook"
NS = {"d": DOCBOOK}
XMLID = "{http://www.w3.org/XML/1998/namespace}id"

# The published tables carry zero-width spaces so long UIDs and keywords wrap in a
# browser. They are presentation, not data.
ZERO_WIDTH_SPACE = "​"


def fetch(path):
    if os.path.exists(path):
        return path
    print("fetching %s" % PART06_URL, file=sys.stderr)
    req = urllib.request.Request(PART06_URL, headers={"User-Agent": "godicom-uid-audit"})
    with urllib.request.urlopen(req, timeout=600) as r, open(path, "wb") as f:
        f.write(r.read())
    print("wrote %s (%d bytes)" % (path, os.path.getsize(path)), file=sys.stderr)
    return path


def cell_text(td):
    return " ".join("".join(td.itertext()).replace(ZERO_WIDTH_SPACE, "").split())


def table_rows(root, table_id):
    for tbl in root.iter("{%s}table" % DOCBOOK):
        if tbl.get(XMLID) == table_id:
            return [
                [cell_text(td) for td in tr.findall("d:td", NS)]
                for tr in tbl.findall(".//d:tbody/d:tr", NS)
            ]
    raise SystemExit("table %s not found in the standard" % table_id)


def read_standard(xml_path):
    """Return {uid: (name, uid_type, keyword, retired)} from PS3.6 A-1 and A-2."""
    root = ET.parse(xml_path).getroot()
    std = {}
    for uid, name, keyword, uid_type in (r[:4] for r in table_rows(root, "table_A-1")):
        # A retired UID is published with "(Retired)" appended to its name; pydicom
        # keeps the two apart, so split them here to compare like with like.
        retired = "(Retired)" in name
        name = " ".join(name.replace("(Retired)", "").split())
        std[uid] = (name, uid_type, keyword, retired)
    for uid, name, keyword in (r[:3] for r in table_rows(root, "table_A-2")):
        std.setdefault(uid, (name, "Well-known frame of reference", keyword, False))
    return std


# Names in the generated file hold no escaped quotes, so matching to the next key
# boundary is exact. If that ever changes this pattern will simply stop matching
# the entry, and the audit will report it as missing rather than silently skip it.
ENTRY = re.compile(
    r'"([\d.]+)": \{Name: "(.*?)", Type: "(.*?)", '
    r'ExtraInfo: "(.*?)", Retired: (true|false), Keyword: "(.*?)"\},'
)


def read_generated(go_path):
    """Return {uid: (name, uid_type, keyword, retired)} from the generated Go."""
    with open(go_path, encoding="utf-8") as f:
        src = f.read()
    got = {}
    for m in ENTRY.finditer(src):
        got[m.group(1)] = (m.group(2), m.group(3), m.group(6), m.group(5) == "true")
    if not got:
        raise SystemExit("parsed no entries out of %s" % go_path)
    return got


def numeric(uid):
    return [int(part) for part in uid.split(".")]


def main():
    script_dir = os.path.dirname(os.path.abspath(__file__))
    xml_path = sys.argv[1] if len(sys.argv) > 1 else os.path.join(script_dir, "part06.xml")
    std = read_standard(fetch(xml_path))
    got = read_generated(os.path.join(script_dir, "uid", "dictionary_generated.go"))

    print("standard A-1 + A-2: %d UIDs; generated dictionary: %d" % (len(std), len(got)))

    problems = 0

    missing = sorted(set(std) - set(got), key=numeric)
    if missing:
        problems += len(missing)
        print("\n%d registered UIDs are absent from the dictionary:" % len(missing))
        for uid in missing:
            name, uid_type, keyword, retired = std[uid]
            print("  %s  %s  %s  retired=%s  %s" % (uid, uid_type, keyword, retired, name))
        print(
            "\nAdd them to STANDARD_ADDITIONS in generate_uid_dict.py if pydicom\n"
            "still lacks them, or bump the pydicom submodule if it has caught up."
        )

    extra = sorted(set(got) - set(std), key=numeric)
    if extra:
        problems += len(extra)
        print("\n%d dictionary UIDs are not in A-1 or A-2:" % len(extra))
        for uid in extra:
            print("  %s  %s" % (uid, got[uid][0]))

    # Names are compared loosely on purpose: pydicom splits the text after a colon
    # into its own field, and edits wording between editions. Keyword and retired
    # are what consumers resolve against, so those have to match exactly.
    for uid in sorted(set(std) & set(got), key=numeric):
        _, _, std_keyword, std_retired = std[uid]
        _, _, got_keyword, got_retired = got[uid]
        if std_keyword != got_keyword:
            problems += 1
            print("\n%s keyword: standard %r, generated %r" % (uid, std_keyword, got_keyword))
        if std_retired != got_retired:
            problems += 1
            print("\n%s retired: standard %s, generated %s" % (uid, std_retired, got_retired))

    if problems:
        print("\n%d discrepancies" % problems)
        return 1
    print("no discrepancies")
    return 0


if __name__ == "__main__":
    sys.exit(main())
