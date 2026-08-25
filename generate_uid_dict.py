"""Generate Go UID dictionary from pydicom's _uid_dict.py, plus STANDARD_ADDITIONS."""
import ast
import os
import subprocess

NATIVE_ENCODING = {
    "1.2.840.10008.1.2",
    "1.2.840.10008.1.2.1",
    "1.2.840.10008.1.2.2",
    "1.2.840.10008.1.2.1.99",
}

GO_KEYWORDS = {
    "break",
    "case",
    "chan",
    "const",
    "continue",
    "default",
    "defer",
    "else",
    "fallthrough",
    "for",
    "func",
    "go",
    "goto",
    "if",
    "import",
    "interface",
    "map",
    "package",
    "range",
    "return",
    "select",
    "struct",
    "switch",
    "type",
    "var",
}

MANUAL_CONSTS = {
    "NativePixels",
    "PYDICOMImplementationUID",
    "GodicomImplementationUID",
    "JPEGBaseline",
    "JPEGExtended",
    "JPEGLSLossy",
    "VerificationSOPClass",
}

# UIDs that PS3.6 Table A-1 registers but pydicom's _uid_dict.py does not carry.
#
# The dictionary is otherwise a 1:1 mirror of pydicom's, and staying a mirror is
# the point -- it is what lets `uid` be checked against the same source as the
# rest of the port. These are the exception because the standard, not pydicom, is
# the authority on which UIDs exist: a consumer resolving a Storage SOP Class it
# received over the wire cannot wait for pydicom to catch up.
#
# Established by diffing the whole registry rather than by spotting them one at a
# time. Parsing Table A-1 and A-2 out of part06.xml and comparing against
# uid/dictionary_generated.go gave 496 registered UIDs against 490 generated
# ones: these six were the entire difference, with no extra UIDs on the Go side
# and no name, keyword or retired-flag disagreement anywhere else. Absent from
# pydicom both at the pinned submodule and at upstream main 0e98c4a (2026-08-01).
#
# Values use the same 5-tuple shape as pydicom's UID_dictionary, so the merge
# needs no special casing downstream:
#     (name, uid_type, extra_info, retired, keyword)
# extra_info is the text pydicom splits off after a colon in the registered name
# (as in "Implicit VR Little Endian: Default Transfer Syntax for DICOM"); none of
# these names have one. retired is pydicom's literal "Retired" or "".
STANDARD_ADDITIONS = {
    "1.2.840.10008.5.1.4.1.1.2.3": (
        "CT Image Storage - For Processing",
        "SOP Class",
        "",
        "",
        "CTImageStorageForProcessing",
    ),
    "1.2.840.10008.5.1.4.1.1.2.4": (
        "Enhanced CT Image Storage - For Processing",
        "SOP Class",
        "",
        "",
        "EnhancedCTImageStorageForProcessing",
    ),
    "1.2.840.10008.5.1.4.1.1.2.5": (
        "Legacy Converted Enhanced CT Image Storage - For Processing",
        "SOP Class",
        "",
        "",
        "LegacyConvertedEnhancedCTImageStorageForProcessing",
    ),
    "1.2.840.10008.5.1.4.1.1.9.100.1": (
        "Waveform Presentation State Storage",
        "SOP Class",
        "",
        "",
        "WaveformPresentationStateStorage",
    ),
    "1.2.840.10008.5.1.4.1.1.9.100.2": (
        "Waveform Acquisition Presentation State Storage",
        "SOP Class",
        "",
        "",
        "WaveformAcquisitionPresentationStateStorage",
    ),
    "1.2.840.10008.5.1.4.1.1.601.5": (
        "Ultrasound Waveform Storage",
        "SOP Class",
        "",
        "",
        "UltrasoundWaveformStorage",
    ),
}


def parse_uid_dict(filepath):
    with open(filepath, encoding="utf-8") as f:
        tree = ast.parse(f.read())

    uid_dict = {}
    for node in ast.walk(tree):
        targets = []
        value = None
        if isinstance(node, ast.Assign):
            targets = node.targets
            value = node.value
        elif isinstance(node, ast.AnnAssign):
            targets = [node.target]
            value = node.value

        if value is None or not isinstance(value, ast.Dict):
            continue

        for target in targets:
            if isinstance(target, ast.Name) and target.id == "UID_dictionary":
                for k, v in zip(value.keys, value.values):
                    uid = ast.literal_eval(k)
                    if isinstance(v, ast.Tuple) and len(v.elts) == 5:
                        name = ast.literal_eval(v.elts[0])
                        uid_type = ast.literal_eval(v.elts[1])
                        info = ast.literal_eval(v.elts[2])
                        retired = ast.literal_eval(v.elts[3])
                        keyword = ast.literal_eval(v.elts[4])
                        uid_dict[uid] = (name, uid_type, info, retired, keyword)

    return uid_dict


def escape_go_string(s):
    s = s.replace("\\", "\\\\")
    s = s.replace('"', '\\"')
    s = s.replace("\n", "\\n")
    s = s.replace("\t", "\\t")
    return s


def is_valid_go_ident(name):
    if not name or name in GO_KEYWORDS:
        return False
    if not (name[0].isalpha() or name[0] == "_"):
        return False
    return all(c.isalnum() or c == "_" for c in name)


def merge_standard_additions(uid_dict):
    """Fold STANDARD_ADDITIONS into the dictionary parsed from pydicom.

    Everything that could go wrong with an addition is checked here, before any
    output is written, so a bad entry cannot leave a half-regenerated tree behind.

    Refusing to run once pydicom defines one of these is deliberate. Skipping it
    silently would leave a second definition of a UID pydicom now owns, free to
    drift from it forever without anyone noticing -- which is the failure this
    table exists to avoid, not to create.
    """
    caught_up = sorted(set(STANDARD_ADDITIONS) & set(uid_dict))
    if caught_up:
        raise SystemExit(
            "pydicom now defines %s.\n"
            "Drop them from STANDARD_ADDITIONS and re-run: once pydicom has an "
            "entry, pydicom is the source of truth for it." % ", ".join(caught_up)
        )

    # The constant loop skips a keyword that is empty, already taken, reserved to
    # MANUAL_CONSTS, or not a valid Go identifier. Those skips are long-settled for
    # pydicom's entries, but for an addition the same skip would put a UID in the
    # dictionary and quietly ship no constant for it. Checking the four conditions
    # up front is equivalent, and reports which one failed.
    taken = {entry[4] for entry in uid_dict.values() if entry[4]}
    seen = {}
    for uid, entry in sorted(STANDARD_ADDITIONS.items()):
        keyword = entry[4]
        if not is_valid_go_ident(keyword):
            raise SystemExit("%s: %r is not a valid Go identifier" % (uid, keyword))
        if keyword in MANUAL_CONSTS:
            raise SystemExit(
                "%s: keyword %s is reserved to MANUAL_CONSTS" % (uid, keyword)
            )
        if keyword in taken:
            raise SystemExit(
                "%s: keyword %s is already used by a pydicom entry" % (uid, keyword)
            )
        if keyword in seen:
            raise SystemExit(
                "%s and %s both use keyword %s" % (seen[keyword], uid, keyword)
            )
        seen[keyword] = uid

    uid_dict.update(STANDARD_ADDITIONS)


def main():
    script_dir = os.path.dirname(os.path.abspath(__file__))
    uid_dict_path = os.path.join(
        script_dir, "pydicom", "src", "pydicom", "_uid_dict.py"
    )
    uid_dict = parse_uid_dict(uid_dict_path)
    merge_standard_additions(uid_dict)

    dict_lines = []
    dict_lines.append("package uid")
    dict_lines.append("")
    dict_lines.append("// Code generated by generate_uid_dict.py. DO NOT EDIT.")
    dict_lines.append("")
    dict_lines.append("// DictEntry holds metadata for a registered DICOM UID.")
    dict_lines.append("type DictEntry struct {")
    dict_lines.append("\tName      string")
    dict_lines.append("\tType      string")
    dict_lines.append("\tExtraInfo string")
    dict_lines.append("\tRetired   bool")
    dict_lines.append("\tKeyword   string")
    dict_lines.append("}")
    dict_lines.append("")
    dict_lines.append("// Dictionary maps UID values to their metadata.")
    dict_lines.append("var Dictionary = map[string]DictEntry{")
    for uid in sorted(uid_dict.keys()):
        name, uid_type, info, retired, keyword = uid_dict[uid]
        retired_bool = "true" if retired == "Retired" else "false"
        dict_lines.append(
            f'\t"{escape_go_string(uid)}": {{Name: "{escape_go_string(name)}", '
            f'Type: "{escape_go_string(uid_type)}", ExtraInfo: "{escape_go_string(info)}", '
            f'Retired: {retired_bool}, Keyword: "{escape_go_string(keyword)}"}},'
        )
    dict_lines.append("}")
    dict_lines.append("")
    dict_lines.append("// KeywordToUID maps UID keywords to values.")
    dict_lines.append("var KeywordToUID = map[string]UID{")
    for uid in sorted(uid_dict.keys()):
        keyword = uid_dict[uid][4]
        if keyword == "":
            continue
        dict_lines.append(f'\t"{escape_go_string(keyword)}": "{escape_go_string(uid)}",')
    dict_lines.append("}")

    dict_path = os.path.join(script_dir, "uid", "dictionary_generated.go")
    with open(dict_path, "w", encoding="utf-8") as f:
        f.write("\n".join(dict_lines) + "\n")

    const_lines = []
    const_lines.append("package uid")
    const_lines.append("")
    const_lines.append("// Code generated by generate_uid_dict.py. DO NOT EDIT.")
    const_lines.append("")
    const_lines.append("const (")
    seen_keywords = set()
    for uid in sorted(uid_dict.keys()):
        keyword = uid_dict[uid][4]
        if keyword == "" or keyword in seen_keywords or keyword in MANUAL_CONSTS:
            continue
        if not is_valid_go_ident(keyword):
            continue
        seen_keywords.add(keyword)
        const_lines.append(f'\t{keyword} UID = "{escape_go_string(uid)}"')
    const_lines.append(")")

    const_path = os.path.join(script_dir, "uid", "generated.go")
    with open(const_path, "w", encoding="utf-8") as f:
        f.write("\n".join(const_lines) + "\n")

    # CI's gofmt gate skips '*_generated.go', which "generated.go" does not match,
    # so this file has to be formatted. The constants are emitted one per line with
    # a single tab and gofmt turns them into an aligned block; doing it here rather
    # than leaving it to whoever runs the script keeps a regeneration from showing
    # up as a ~1000-line whitespace diff on top of the real change.
    #
    # dictionary_generated.go is left alone on purpose: it does match the exclusion
    # and has always been committed unformatted, so formatting it now would realign
    # a 900-entry map literal for no benefit.
    subprocess.run(["gofmt", "-w", const_path], check=True)

    print(f"Generated {dict_path} with {len(uid_dict)} UIDs")
    print(f"Generated {const_path} with {len(seen_keywords)} constants")


if __name__ == "__main__":
    main()
