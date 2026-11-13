# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- `DirectoryRecords` builds the directory records a DICOMDIR holds for one SOP
  instance: the PATIENT/STUDY/SERIES/leaf chain for an ordinary instance, or a
  single record for the SOP Classes DICOM files directly under the File-set root
  (PALETTE, IMPLANT, HANGING PROTOCOL and the rest), chosen the way pydicom
  chooses them — Modality and the instance's own attributes first, then the SOP
  Class table. An instance that is missing an element PS3.3 Section F.5 requires
  at its level, or that has one with no value, is refused rather than filed
  under a record with holes. This is the record-creating half of the File-set
  writer; the DICOMDIR the records go into is written by the entries below
- `GenerateFilename` and `IsConformantFileID` cover the File ID rules of PS3.10
  Sections 8.2 and 8.5: eight characters per component, eight components, and
  [0-9][A-Z] and '_' only. `GenerateFilename` yields numeric or alphanumeric
  suffixes, and like pydicom it leaves J out of the alphanumeric set
- `FileSet.WriteDICOMDIR` and `FileSet.EncodeDICOMDIR` write the DICOMDIR
  itself: the records are laid out depth first, each one's *Offset of the Next
  Directory Record* and *Offset of Referenced Lower Level Directory Entity* set
  to where the record it links to landed, and the root's first and last record
  pointers filled in last. `FileSet.Write` replaces the DICOMDIR of the File-set
  it was read from. The result is byte for byte what pydicom 3.0.0's
  `FileSet._write_dicomdir()` produces for the same File-set, in explicit VR
  and with `ForceImplicitVR` alike, from the reordered, no-offset, big endian
  and implicit VR DICOMDIR test fixtures as well as the conformant one
- `NewFileSet`, `Add`, `AddFile`, `Remove` and `Save` build a File-set instead
  of only reading one: an instance is filed under the PATIENT/STUDY/SERIES
  records its SOP Instance UID and Study/Series UIDs put it in, sharing the
  records the File-set already has, and `Save` writes each instance at the File
  ID its position implies (PT000000/ST000000/SE000000/IM000000 and so on) and
  then the DICOMDIR. A File-set built this way opens in pydicom with the same
  records and File IDs pydicom itself would give it. `Save` refuses to move an
  existing File-set to another root -- that is `CopyTo` -- and `UseExisting`
  keeps the layout a File-set was read with and rewrites only the DICOMDIR
- `AddCustom` files an instance under directory records the caller supplies,
  which is the only way to write a PRIVATE record: it carries nothing that says
  where it belongs, so no recorder could derive one. The chain the leaf hangs
  from joins the tree where it meets it, the three Referenced UID elements are
  filled in from the instance, and the records are checked the way pydicom
  checks them -- a Directory Record Type, the element that identifies the
  record among its siblings, and no more than eight levels
- `CopyTo` copies a File-set to another root and returns the File-set that
  lives there, laid out at the File IDs the records imply, with the original
  left exactly as it was. The DICOMDIR it writes is the same one pydicom's
  `FileSet.copy()` writes for the same File-set, and the destination has to be
  a new one: an existing DICOMDIR or an occupied File ID is a refusal
- Orphaned directory records -- records the offset links never reach -- are
  filed back in by reading each instance they name and rebuilding the records
  it calls for, which is pydicom's default too. `FileSetOptions.ExcludeOrphans`
  leaves them out instead, and `RaiseOrphans` still reports them

## [0.37.0] - 2026-11-13

Nothing in the library changed. What this release accepts and what it writes is
what `v0.36.0` did; the work in it is the test suite and the coverage reporting
catching up.

Two tests now cover behaviour that had none. Writing through a symlink still
updates the file the link points at and leaves the link a link — a rename would
have replaced the name instead — and the test compares the inode of the target
before and after, so it tells those two implementations apart rather than
passing either way. And every VR in `StandardVRs` is written and read back, so
a VR added without a sample value fails with `no sample value for <VR>` instead
of being skipped quietly.

### Changed
- Coverage is published again on every push to `main`, so the badge in
  `README.md` shows a percentage instead of `unknown`. The upload step had been
  in the workflow all along but never received the `CODECOV_TOKEN` secret, so
  Codecov turned every report away with "Token required because branch is
  protected" — and the step is non-fatal, so the job stayed green and the
  failure never showed

## [0.36.0] - 2026-11-13

### Changed
- `SaveAs`, `WriteFile` and `WriteFileContext` write to a temporary file beside
  the target and rename it onto the target only once the whole write succeeded.
  Every error the writer can raise after `os.Create` — a value the VR cannot
  hold, a strict `OnDiagnostic` hook, a character set changed under an
  un-decoded element, a full disk — used to leave a truncated file where a good
  one had been, and a truncated DICOM file still opens, so nothing downstream
  noticed. A failed write now leaves the previous file as it was, or no file at
  all where there was none. Two consequences worth knowing: the directory has to
  be writable, which truncating an existing file did not need, and a hard link
  to the old file keeps the old contents. A path that is a symlink, a device or
  a pipe is still written through in place, because a rename would replace the
  name rather than the thing it pointed at

### Fixed
- Writing an integer that its VR cannot hold is refused instead of truncated:
  a `US` set to 70000 used to reach the file as 4464, and `US` with -1 as
  65535. `US`/`SS` now check the 16-bit range and `UL`/`SL` the 32-bit one,
  naming the VR and the tag in the error. pydicom refuses the same values at
  pack time (`'H' format requires 0 <= number <= 65535`). This is a behaviour
  change: the call fails where it used to succeed with a different number
- A value whose Go type has no encoding for its VR is refused instead of
  written as a zero-length element. `AT` holding a `uint32`, `OB` holding a
  `uint32` and `OW` holding a `[]uint16` all used to disappear without an
  error; pydicom raises `TypeError` for the last one. The error names the
  dynamic type, the VR and the tag. The types a VR does accept were widened in
  the same pass — `int8`, `int16`, `uint8`, `[]uint16`, `[]uint32`, `[]uint64`
  for the integer VRs — so a caller who wrote the natural Go type is not
  rejected for it

## [0.35.0] - 2026-11-12

### Added
- Enhanced multi-frame functional groups are read. An enhanced CT/MR/PT
  instance keeps rescale, window, pixel measures and palette inside
  `(5200,9229)` / `(5200,9230)` rather than at the top level, so those frames
  were rendered with the right shape but the wrong contrast and physical scale.
  Resolution is per-frame → shared → top level, and the existing dataset-wide
  entry points keep their old behaviour (shared + top only) so no signature
  changes. New per-frame APIs: `FileDataset.PixelMeasures(frameIndex)`,
  `ApplyModalityLUTForFrame`, `ApplyVOILUTForFrame`,
  `ApplyPresentationLUTShapeForFrame` and `ApplyColorLUTForFrame`.
  `DisplayFrame` now resolves its parameters for the frame it is given.
  `ApplyColorLUTForFrame` also reads a table carried inline in a functional
  group item, or selects one from the Enhanced Palette Color Lookup Table
  Sequence `(0028,140B)` by the frame's
  `Palette Color Lookup Table UID (0028,1199)`. A missing or unmatched UID is
  an error rather than a silently wrong colour. pydicom has no equivalent —
  its handlers read top-level elements — so the precedence and the per-frame
  API shape follow PS3.3 C.7.6.16 rather than a port
- `pixels.FrameCountSource` is an optional interface for a dataset that can
  count frames from `(5200,9230)`. `DescriptorFromDataset` uses it when
  `NumberOfFrames (0028,0008)` is absent, instead of silently decoding one
  frame of a multi-frame image. `FileDataset` implements it
- `DiagnosticUnknownCharacterSet` is reported once per `(0008,0005)` element
  naming a character set godicom does not implement. `Err` keeps every
  unresolved value with its index. The historical `ISO_IR 6` fallback still
  applies, so a read without an `OnDiagnostic` hook behaves exactly as before
  apart from a warn-level log line. An archiving endpoint can now refuse the
  file instead of silently storing a GB18030 Chinese name as mojibake
- `DiagnosticInvalidCharacterData` is reported when a text VR cannot be decoded
  with the declared character set. `RawValue` keeps the undecoded bytes
- `ReadOptions.ReportNonASCIIUnderDefaultCharset` makes the reader report a text
  value carrying bytes above `0x7F` while the default repertoire is in effect —
  `ISO_IR 6`, or no `(0008,0005)` at all — as `DiagnosticMisdeclaredCharacterSet`,
  once per dataset with the first offending element. This is the declaration
  older Chinese and Japanese equipment ships without saying so, and it used to
  read as mojibake with nothing reported. It is off by default: the same bytes
  also occur in Latin-1 free text that archives without complaint, and turning it
  on would reject studies that read exactly as they always did under any hook
  that fails closed. pydicom does not detect the case at all
- `Dataset.ReinterpretCharacterSet` decodes the text of a dataset that was read
  from bytes using the set the caller declares, and updates `(0008,0005)` to
  match. That is the repair for a file whose declaration is wrong, which is the
  common case for older Chinese and Japanese equipment: the value still holds the
  bytes of the file, so re-decoding recovers the original name, where re-encoding
  turns every non-ASCII run into U+FFFD. It covers both `ISO_IR 6` (or absent)
  with national bytes, and a single byte set in front of GBK bytes, where the
  read produced mojibake that nothing reported. Elements the caller has replaced
  since the read are left alone
- `encaps.GetFrameFrom` reads one frame of encapsulated pixel data out of an
  `io.ReaderAt` instead of a `[]byte`, so a frame can be had from a file without
  holding the element. It covers what pydicom's `pixel_data_reader` parameter
  does: with a Basic or Extended Offset Table, locating a frame costs the item
  headers, and with neither it costs the last ten bytes of each fragment, where
  the JPEG EOI marker that delimits frames lives. `GetFrame` now delegates to it,
  so the two produce identical bytes and the identical error for an index out of
  range. Reading frame 5 of 8 costs 12% of the element instead of all of it
- `godicom.ValidateValue` checks one string value against PS3.5 Table 6.2-1 for
  every VR that defines a format — the length cap, the characters the VR allows,
  and the shapes `DA`, `TM`, `DT`, `AS`, `UI` and `PN` have to have — and returns a `*ValueValidationError` naming each rule the
  value breaks. It is the same set of 15 rules pydicom keeps in `VALIDATORS`,
  written once instead of once per entry point. The writer runs it for every
  value it is about to encode and reports a failure as
  `DiagnosticInvalidValue`, so a value the reader would reject is no longer
  written out quietly. With no `OnDiagnostic` hook set nothing changes: a round
  trip still writes back what it read, even a value that was never legal, since
  a caller re-saving a file it did not create must not be made to repair it

### Changed
- `DiagnosticInvalidCharacterData` is reported for at most
  `DefaultMaxInvalidCharacterDataDiagnostics` (eight) elements per dataset, and
  then once more as a summary whose `Count` holds how many elements it stands
  for, with the tag, offset and path of the first of them. A dirty study used to
  emit one diagnostic and one warn log line per undecodable value — thousands for
  a real archive — which buried the signal the diagnostics were added for.
  `ReadOptions.MaxInvalidCharacterDataDiagnostics` moves the cap, and a negative
  value reports every element as before. What gets parsed and what a value holds
  are unchanged
- Writing a dataset after changing `(0008,0005)` is refused with
  `ErrCharsetChangeLosesText` when the dataset was read under a set that does
  not decode and a text value still holds non-ASCII bytes from the source. The
  writer used to re-encode those bytes as if they were text, so changing the
  declaration and saving destroyed a Chinese name instead of repairing it. Text
  the caller set in Go is Unicode already and unaffected, ASCII-only text is
  identical in every supported set, and `ReinterpretCharacterSet` is the way to
  change the declaration without loss. `WriteFile` runs the check before it
  creates the file, so a refusal leaves an existing file untouched

### Fixed
- `DisplayFrame` no longer ignores the rescale and window of an enhanced
  multi-frame instance, and no longer renders every frame with frame 0's
  parameters. `needsDisplayLUT` also checks the functional groups before taking
  the "8-bit color is already display-ready" short-circuit. Both failures were
  silent: no error, just a plausible-looking thumbnail
- `Dataset.GetFloats` and `GetFloat` accept `float32` and `MultiValue[float32]`.
  A dataset parsed from the DICOM JSON Model stores an `FL` value as `float32`
  (`coerceParsedValue`), so a caller that read QIDO/WADO metadata and asked for
  `(0018,9431)` got `0, false` even though the element was there. Values the
  binary reader produces were already `float64`, which is why only the JSON path
  was affected
- Marshaling a DICOM JSON Model dataset no longer drops a binary element whose
  bytes are only in `RawValue`. `elementToJSON` returned early on a nil `Value`
  before reaching `binaryValue`, so the `RawValue` branch it exists for was
  unreachable; an element restored from its source bytes serialised as a bare
  `{"vr":"OB"}` with no `InlineBinary`
- CS padding in a multi-valued *Specific Character Set*
  (`"GB18030 \ISO_IR 100"`) was not stripped from each value, so the first term
  failed lookup and the whole dataset fell back to `ISO_IR 6`. Each value is now
  trimmed at both ends. This is a difference from pydicom, not a copy of it:
  pydicom only rstrips the whole value (`multi_string` in `pydicom/values.py`)
  and gets away with it because Python's codec registry normalises `"GB18030 "`
  to `gb18030` at lookup time. A Go map lookup has no such fallback
- `ISO_IR 13`, `ISO 2022 GBK` and `ISO 2022 58` were missing from the character
  set table, so a file declaring one of them was reported as an unknown
  character set and then decoded as `ISO_IR 6`. All three are defined terms
  (PS3.5 Table C.12-2 and CP1234) and pydicom decodes them as `shift_jis`,
  `GBK` and `GB2312` respectively. The table now matches pydicom's
  `python_encoding` entry for entry, and `TestCharacterSetTableMatchesPydicom`
  fails if a term goes missing again
- Spelling variants of defined terms (`ISOIR192`, `GB-18030`, `UTF-8`, case
  differences) resolve through a case-insensitive alias table built from the
  supported encoding map. `ISOIR192` previously produced `ISO_IR192`, which is
  not a supported key and silently decoded as `encoding.Nop`
- Decode errors in `DecodeBytesWithDelimiters` are no longer swallowed. The
  exported function keeps its loose behaviour; the reader uses a checked
  counterpart, so invalid UTF-8 under `ISO_IR 192` and bytes a charmap decoder
  replaced with U+FFFD are reported rather than looking like valid text
- A U+FFFD that is actually in the file is no longer mistaken for a decode
  failure. Decoding valid UTF-8 cannot substitute anything, and GB18030 can
  encode U+FFFD, so the codec is now asked which of the two happened instead of
  scanning the output for the replacement character. A strict `OnDiagnostic`
  hook used to reject a legal file whose text contained U+FFFD
- `tag.Parse` rejected every tag with the high bit set in its group or element.
  The bare eight-digit form was read with `strconv.ParseInt(s, 16, 32)`, which
  overflows at `0x80000000`, so `"FFFEE000"` — the key `Tag.JSONKey` emits and
  the key a DICOM JSON dataset uses for an item tag — came back as
  `tag: unknown keyword`. PS3.5 §9.1 defines a tag as four *unsigned* hex
  digits, and godicom's own `ParseTag` already read it that way
- A DICOM JSON `UN` value carrying an undefined-length sequence was mis-parsed
  when the payload itself contained the bytes `FE FF DD E0`. The item and
  sequence parsers scanned the raw buffer for the delimiter after they had
  finished, so a pixel fragment or a private string ending in those four bytes
  ended the sequence early and the rest was reported as
  `element (0100,0002) length exceeds dataset`. Both parsers now return the
  offset where they stopped, which is what the scan was looking for anyway

## [0.34.0] - 2026-10-23

### Changed
- A `dicomjson.BulkDataURIBuilder` that returns an error or an empty URI writes
  that value as InlineBinary. Pixel Data can stay a BulkDataURI while other
  binary elements, including private `UN` values, remain in the document

## [0.33.0] - 2026-10-20

### Added
- `OpenFileSet` and `LoadFileSet` read a DICOMDIR into its directory-record
  tree and resolve each Referenced File ID against the directory that contains
  the DICOMDIR. `Find` searches those records. Writing a DICOMDIR, adding or
  removing instances, and pulling orphaned records back in are not included
- `Dataset.OverlayArray` unpacks `(60xx,3000)` *Overlay Data* into a 0/1 bit
  plane. The group runs from `0x6000` to `0x60FF`, and a missing Number of
  Frames in Overlay means one frame

## [0.32.0] - 2026-10-20

### Added
- `ApplyColorLUT` maps stored pixels through a Palette Color Lookup Table,
  including segmented tables and the eight PS3.6 well-known palettes (`PET`,
  `HOT_IRON`, `SPRING`, and the rest, by name or SOP Instance UID).
  `DisplayFrame` uses it for `PALETTE COLOR` images, packing 16-bit entries
  down to the high byte. Supplemental and enhanced palette modules are not
  supported

## [0.31.0] - 2026-10-20

### Added
- `ReadOptions.OnElement` decides, per element, whether to parse its value, step
  over it, or stop reading — so a caller after seven index tags out of a 200 MB
  file no longer pays to decode the other 199 MB. The hook is handed a
  `RawDataElement` (Tag, VR, Length, ValueTell; `Value` is always nil, not having
  read it being the point) and the `[]PathStep` naming the enclosing sequences,
  and returns `ElementKeep`, `ElementSkip` or `ElementStop`. It is called at every
  depth, so an oversized nested sequence can be stepped over element by element —
  something `SpecificTags` cannot express at all. The saving is real on a seekable
  source, where the value bytes are never read; on a non-seekable reader godicom
  has already buffered the stream, so a skip saves the decode and the allocation
  but not the I/O. Group 0x0002 and (0008,0005) Specific Character Set are never
  offered, because skipping either changes what the rest of the file *means*
  rather than how much of it is kept
  ([#72](https://github.com/godicom-dev/godicom/issues/72))

### Fixed
- **BREAKING: `SpecificTags` no longer empties the sequences it keeps.** It
  filtered at every depth, so `SpecificTags: []Tag{seqTag}` kept the sequence
  element and dropped every element inside its items — `(3006,0010)` came back
  with one item holding zero of its two elements, a dataset no file could have
  produced. It now filters the top-level dataset only, which is where pydicom
  applies it (`read_sequence_item` calls `read_dataset` without passing
  `specific_tags` along), and a sequence that survives the filter is parsed whole.
  Pass an `OnElement` hook that returns `ElementSkip` when `len(path) > 0` for the
  previous behaviour

### Changed
- `StopBeforePixels` and `SpecificTags` are implemented on the one decision point
  `OnElement` runs at, rather than as twelve separate checks scattered across the
  three element loops, so the two byte-slice loops and the `ReaderAt` loop agree on
  what a read keeps by construction. `StopBeforePixels` is unchanged:
  `TestOnElementStopMatchesStopBeforePixels` holds a hook stopping at `(7FE0,0010)`
  against the option and compares the datasets tag for tag. The `SpecificTags`
  change is the one under Fixed. Skipping an element
  does not suppress the diagnostics for it — they describe the file, not the subset
  a caller kept — but `ElementStop` ends the parse, so nothing past it is reported

## [0.30.0] - 2026-09-01

### Added
- Six Storage SOP Class UIDs that PS3.6 Table A-1 registers but pydicom's
  `_uid_dict.py` does not carry, so `uid.LookupKeyword` and `UID.Name` resolve them
  instead of returning the raw UID string: `CTImageStorageForProcessing`
  (`1.2.840.10008.5.1.4.1.1.2.3`), `EnhancedCTImageStorageForProcessing` (`.2.4`),
  `LegacyConvertedEnhancedCTImageStorageForProcessing` (`.2.5`),
  `WaveformPresentationStateStorage` (`1.2.840.10008.5.1.4.1.1.9.100.1`),
  `WaveformAcquisitionPresentationStateStorage` (`.100.2`) and
  `UltrasoundWaveformStorage` (`1.2.840.10008.5.1.4.1.1.601.5`). The UID
  dictionary is otherwise a 1:1 mirror of pydicom's and stays one; these are
  supplied from a `STANDARD_ADDITIONS` table in `generate_uid_dict.py`, which
  refuses to run once pydicom defines any of them so the two cannot silently
  drift. Reported in [#77](https://github.com/godicom-dev/godicom/issues/77),
  which named the two Waveform Presentation State classes; a full diff of the
  registry found the other four
- `audit_uid_dict.py` compares the generated dictionary against PS3.6 Tables A-1
  and A-2 — it fetches the standard, then reports UIDs missing from the
  dictionary, UIDs the standard does not register, and any keyword or retired-flag
  disagreement. It is how the six above were found, and it is deliberately not in
  CI: it depends on the network and on a document that changes independently of
  this repository, so a new DICOM edition would otherwise turn an unrelated pull
  request red. Run it after bumping the pydicom submodule
- `Dictionary`, a one-method interface over the data dictionary —
  `Lookup(tag Tag, creator string) (DictEntry, bool)` — and `Standard()`, which
  returns the dictionary PS3.6 defines. One method answers for both halves of the
  dictionary: `creator` names the Private Creator of a private tag and is ignored
  for a standard one. `Lookup` resolves an exact entry ahead of a repeating-group
  pattern, which is not arbitrary — `(7FE0,0010)` is Pixel Data exactly and retired
  Variable Pixel Data through the `7Fxx,0010` mask — and never resolves a private
  tag against the standard tables, because the same tag means different things to
  different vendors. The package's own lookups go through it, so it is the path
  godicom itself takes rather than a wrapper around one; the type is empty and its
  method set is known, so the compiler inlines `Standard` and devirtualizes each
  internal call back into a map access. It is also the first public way to read a
  standard entry's VM, name or retired flag: `LookupVR` gave the VR and laundered a
  missing entry into `UN`, and the `PrivateDictionary*` functions covered only
  private tags. Groundwork for
  [#71](https://github.com/godicom-dev/godicom/issues/71). Documented in the
  README with `ExampleStandard`, `ExampleDictEntry_VRs` and
  `ExampleAddPrivateDictEntry` behind it, so the snippets are compile-checked
- `DictEntry.VRs()` returns the VRs an entry permits, splitting the compound forms
  PS3.6 writes as prose. Thirty-seven entries permit two — `OB or OW` is PixelData,
  so nearly every image ever written — and `(0028,1200)` Gray Lookup Table Data
  permits three, `US or SS or OW`. `vrDisagreesWithDictionary` now uses it instead
  of splitting the string itself
- `ReadOptions.Dictionary` resolves tags for one read, defaulting to `Standard()`
  when nil. It changes what an element *means* rather than how it is described: an
  implicit VR file carries no VRs, so every element's VR is whatever the dictionary
  says, and for a private element that answer depends on the vendor who wrote the
  file. The dataset that comes back retains it, because a deferred value is decoded
  on `Get` — long after the read returned — and the reload rejects an element whose
  VR no longer matches the one it was first read under; a dictionary that went out
  of scope with the parse would turn every deferred private element into a mismatch
  error. It is also carried into the second read a Deflated transfer syntax starts,
  so a file means the same thing deflated and inflated. There is deliberately no
  `WriteOptions.Dictionary`: the write path resolves an ambiguous VR from the
  dataset's own values — Pixel Representation decides between US and SS — and never
  consults the dictionary, so the field would have nothing to do. `DecodeDataset`
  and `DecodeDatasetContext` take no options at all and so still resolve against
  `Standard()`. Closes
  [#71](https://github.com/godicom-dev/godicom/issues/71)
- `NewDictionary(dicts ...Dictionary) Dictionary` composes dictionaries, trying each
  in turn and returning the first entry found, so the argument order is the
  precedence: `NewDictionary(vendor, godicom.Standard())` reads the vendor's entries
  where it has them and PS3.6's everywhere else. First match wins rather than most
  specific, because that is the only rule a caller can read off the argument order.
  Composing rather than replacing is the point — a caller adding one vendor block
  should not have to carry the other 5,189 entries. A nil member is skipped and an
  empty composition answers nothing, both of which a caller threading a built-up
  list will produce; the members are held rather than copied, so a `PrivateDictionary`
  among them stays live, while the slice itself is copied
- `NewPrivateDictionary` returns a `*PrivateDictionary`, a set of private entries
  keyed by Private Creator that satisfies `Dictionary`. `Add(creator, tag, vr, name,
  vm...)` mirrors `AddPrivateDictEntry` so moving from one to the other is
  mechanical, and stores the entry against the tag's block byte, so it resolves
  wherever the vendor's block lands — `(0041,1001)` in one file and `(0041,2001)` in
  the next. Unlike `AddPrivateDictEntry` it touches no process-global state: a
  library can build one, compose it with `Standard()`, and hand it to
  `ReadOptions.Dictionary` without changing what any other caller in the process
  reads. Safe for concurrent use. Documented in the README with
  `ExampleNewPrivateDictionary` behind it, so the snippet is compile-checked
- `Dataset.NewPrivateBlock(group uint16, creator string) (*PrivateBlock, error)`
  returns the block a Private Creator holds in a group, reserving one when it holds
  none. It is what writing private data needs and `PrivateBlock` alone could not do:
  with no `(gggg,00xx)` creator element there is no block, and a private element
  written without one is an element no reader can attribute to any vendor. Reserving
  takes the lowest free block, so a second vendor in the same group lands at `0x11`
  rather than on top of the first, and writes the creator as `LO`, the VR PS3.5
  §7.8.1 gives it. An existing block is returned as it stands and nothing is
  written. It reports an error rather than reserving when the group is even (a
  standard group has no private blocks), when the creator is empty (a block reserved
  under no name is one nothing can resolve), and when all 240 blocks in the group are
  taken — the last cannot happen in a file anyone has written, but the alternative is
  silently returning block `0x00`, which PS3.5 reserves. Mirrors pydicom's
  `Dataset.private_block(..., create=True)`. Part of
  [#51](https://github.com/godicom-dev/godicom/issues/51) §18
- `Dataset.PrivateCreators(group uint16) []string` lists the vendor names that have
  reserved a block in a group, ordered by the block they occupy. It is where to start
  with a file from a manufacturer whose documentation you do not have: the private
  tags say nothing, but the creator names say who to ask. An even group has no
  private blocks and so returns nothing. Mirrors pydicom's
  `Dataset.private_creators`
- `PrivateBlock.Delete(offset uint8)` removes an element from a block, completing the
  set `Get` and `Set` started. It does not release the block — the creator element
  stays, so an emptied block is still that vendor's and not free for another. Mirrors
  pydicom's `PrivateBlock.__delitem__`
- `PrivateBlock` and the PS3.5 §7.8.1 rules it implements now live in
  `private_block.go`, headed by the reason the type exists at all: a private
  element's tag is not fixed by the vendor's documentation, so code that hardcodes
  `(0019,1001)` reads a different vendor's data the first time it meets a file where
  the blocks landed differently — and reads it *successfully*, because there is
  nothing about the bytes to object to. The README has a section on it with
  `ExampleDataset_PrivateBlock` and `ExampleDataset_NewPrivateBlock` behind it, so
  the snippets are compile-checked
- `uid.LookupKeyword(keyword string) (UID, bool)` resolves a PS3.6 keyword to its
  UID — the direction the old `uid.Lookup` went, under a name that says which
  direction that is. It replaces the exported `uid.KeywordToUID` map for the one
  thing a caller could do with it. The generated constants remain the better answer
  for a keyword known at compile time; this is for one that arrives at run time,
  from a configuration file or a command line

### Changed
- **Breaking:** `uid.Lookup` takes a UID and returns what the registry records
  about it — `Lookup(u UID) (Info, bool)` — where it used to take a keyword and
  return a UID. That direction is now `uid.LookupKeyword`, so both exist and each
  says which way it goes. The new `Lookup` is what replaces the exported maps: it
  answers everything `uid.Known` was reachable for, including the four derived
  transfer-syntax flags, and it hands back a copy, so the table cannot be reached
  through it
- **Breaking:** the three exported maps in `uid` are gone from the API —
  `Dictionary` and `KeywordToUID` become `dictionary` and `keywordToUID`, `Known` is
  removed outright (see Removed), and `uid.DictEntry` becomes `dictEntry` with the
  table it types. Same defect as the data dictionary above: read-only by
  convention, mutable by type, no synchronisation of any kind, so any caller could
  have rewritten what a transfer syntax means for every other caller in the
  process — `uid.Known[uid.ImplicitVRLittleEndian]` was one assignment away from
  making every implicit-VR file in the process decode as explicit. Read access is
  `uid.Lookup` and `uid.LookupKeyword`. `generate_uid_dict.py` emits the unexported
  names, so a regeneration cannot quietly export them again. Within the
  godicom-dev organisation the only references are two lines in one gonetdicom
  test, `ae/storage_sop_classes_test.go:35` and `:121`, which a module bump there
  will need to move to `uid.Lookup` / `uid.LookupKeyword`
- **Breaking:** the root aliases `godicom.UIDDictionary` and `godicom.KnownUIDs` are
  removed rather than unexported, there being nothing left for them to alias.
  `godicom.LookupUID` is unchanged and still takes a keyword; `godicom.UIDInfo`
  still names `uid.Info`, which is what `uid.Lookup` returns
- the VR-mismatch diagnostic is judged against the dictionary the read was given
  rather than always against PS3.6. A caller who overrode an entry said what they
  expect the file to contain, and measuring against a different expectation than the
  parse used would be reporting on nothing. Private tags stay exempt even when the
  supplied dictionary has an entry for them: the creator is not in hand at that
  point, and resolving one per element would charge the quiet path for a diagnostic
  nobody asked for
- `AddPrivateDictEntry` and `ResetExtraPrivateDictionaries` now document that they
  mutate process-global state — every caller in the process sees an added entry, and
  `Reset` clears entries registered by code that has nothing to do with the caller —
  and point at `NewPrivateDictionary` plus `ReadOptions.Dictionary` for the version
  that does not. Neither is deprecated: a command-line tool registering its vendor's
  blocks at startup is what they are for. Behaviour is unchanged, and they are now
  implemented on top of `PrivateDictionary` rather than a parallel table
- **Breaking:** the three exported dictionary maps are unexported —
  `DicomDictionaryGo`, `RepeatersDictionaryGo` and `PrivateDictionaries` become
  `dicomDictionary`, `repeatersDictionary` and `privateDictionaries`, and
  `PrivateDictEntry` becomes `privateDictEntry` along with the table it populates.
  They were read-only by convention and mutable by type, with no synchronisation of
  any kind, so any caller could have corrupted the dictionary for the whole
  process. Read access is now `Standard().Lookup`, which answers everything the
  maps were reachable for; `AddPrivateDictEntry` remains the way to add a private
  entry at runtime. Nothing in the godicom-dev organisation referenced the maps.
  `DictEntry` stays exported — it is what `Lookup` returns
- **Breaking:** `Dataset.PrivateBlock` returns `(*PrivateBlock, bool)` rather than a
  bare pointer that was `nil` when no such creator was there. A file from another
  manufacturer has no such block, which is the ordinary case rather than an
  exceptional one, and the old signature made it a nil dereference in the caller's
  next line: `ds.PrivateBlock(0x0019, "GEMS_ACQU_01").Get(0x01)` panics on every file
  GE did not write. The ok idiom matches `Dataset.Get`, and the three situations
  pydicom raises three different exceptions for — even group, empty creator, no such
  creator — collapse into the one answer a caller can act on
- **Breaking:** `group` is `uint16` and `offset` is `uint8` on `Dataset.PrivateBlock`
  and on `PrivateBlock`'s `GetTag`, `Get`, `Set` and `Delete`, where both were `int`;
  `PrivateBlock.Group` is `uint16` for the same reason. A block offset is the low byte
  of an element number and nothing else, and an `int` let it be anything: `GetTag(0x1234)`
  returned `(0009,2234)`, silently a different vendor's block, and `GetTag(-1)`
  returned `(0009,0FFF)`, which is not a private element at all. pydicom raises
  `ValueError` above `0xFF` at run time and does not check for negatives; here the
  type carries the constraint, so the check has nowhere left to fail

### Fixed
- `generate_dict.py` wrote to `dicom_dict_generated.go`, a name nothing in the tree
  has: the checked-in file is `dictionary_generated.go`. Running the generator
  therefore produced a second file redeclaring `DictEntry` and every map in it, so
  the package stopped compiling instead of the dictionary being updated
- both dictionary generators now `gofmt` their output. The checked-in files are
  formatted and the generators' output was not, so regenerating produced a
  5,182-line whitespace diff on top of whatever the data change was
- `godicom`'s usage text listed only `-debug`, having never been updated as `show`
  grew `-no-meta`, `-top`, `-t` and `-tag`. All five are now documented, in the
  usage text and in the README's CLI section. `TestPrintUsageMatchesShowFlags`
  keeps them in step from here: it enumerates the real flag set and fails both on a
  registered flag the usage text omits and on a documented flag that no longer
  exists
- `-top`'s description said "only show top-level elements", which reads as though
  it changes the default output. It does not: plain `show` lists the top level and
  reports a sequence by item count without descending into it either way. The flag
  narrows the `-t` search to the top level, and is a no-op without one. Corrected
  in `show -h`, the usage text and the README. `TestWriteShowTopLevel` asserted
  this against a nil tag filter, which took the non-recursive path regardless, so
  it had been passing without the flag set at all; it now contrasts a filtered
  search with and without `-top`
- `Dataset.PrivateBlock` chose a block by map iteration order when one creator name
  reserved two blocks in a group, so the tag a private element was read from could
  differ between runs of one program over one file. It now scans elements `0x10`
  through `0xFF` in order and the lowest block wins. A well-formed file has no
  duplicate creator, but nothing in the read path rejects one and pydicom has the
  same defect
- a cached `PrivateBlock` outlived the element that reserved it. Deleting a Private
  Creator, or overwriting it with another vendor's name, left the block resolving and
  still answering with its old base element: reads came back from whatever now
  occupies those tags, and writes produced private elements attributed to a vendor the
  dataset no longer names. The cache is now dropped whenever a Private Creator element
  is set or deleted, which covers `Delete`, `Set`, `Pop`, `Clear` and
  `RemovePrivateTags`. pydicom fixed the deletion half in its issue #1097 and still
  caches across the overwrite

### Removed
- the `uid.Known` map and the package `init` that filled it. It was a second copy
  of the UID dictionary — one `Info` per registered UID, all 496 of them built at
  program start whether anything asked or not — and every field in it is either
  read straight from the dictionary entry or derived from the UID in three
  comparisons. `uid.Lookup` derives them on demand instead, so there is one table
  where there were two, nothing to keep in step, and no init work for a program
  that never looks a UID up. `TestLookupCoversEveryEntry` checks every entry
  resolves, which is the invariant `len(Known) == len(Dictionary)` used to state
- `dictionaryIsRetired`, which consulted only the exact table and so answered "not
  retired" for all 72 retired repeating-group entries. It had no caller outside its
  own test, so nothing ever noticed. `Standard().Lookup` returns the entry and
  `Retired` is read off it, which gets the repeaters right;
  `TestLookupReportsRetired` covers one of them
- the generated `tagToKeyword` and `tagToName` maps, 10,380 lines between them.
  Both were a second copy of data `dicomDictionary` already held, reachable by one
  map access; `tagToName` had no reader at all and `keywordForTag` was the only
  reader of `tagToKeyword`. Two 5,000-entry maps are no longer built at init.
  `keywordToTag` stays, because that direction is the one that needs an index.
  staticcheck skips generated files, which is why `tagToName` sat unread without
  `-checks=all` minding
- `var _ = regexp.Compile` in `dictionary.go`, kept by a comment claiming it forced
  an init that the `regexp` package does not have. Nothing in the file used
  `regexp`, so the import went with it

## [0.29.0] - 2026-08-25

### Fixed
- **The data dictionary answered the wrong VR for nearly every tag on a 32-bit
  platform.** The 88 repeater masks were parsed with `fmt.Sscanf` into `int`
  fields with the error discarded; a mask such as `0xFF00FFFF` does not fit in a
  32-bit `int`, so all 88 came out zero, and `(tag ^ value) & 0 == 0` matches
  every tag. Whichever repeater happened to be first in map iteration order
  became the answer for every non-private tag — `LookupVR` returned `US` for
  `PatientName`, group lengths, everything — and `IsRepeaterTag` was true for all
  of them. The masks are now built by nibble arithmetic into `uint32`, which has
  no error to drop and no width to overflow. 64-bit platforms were unaffected
- **`ParseTag` could not parse any tag with a group of `0x8000` or above from its
  hex-string form**, on every platform. It used `strconv.ParseInt(v, 16, 32)`,
  and a signed 32-bit parse rejects everything from `0x80000000` up — so
  `ParseTag("FFFEE000")` failed with `unknown tag keyword` even though the item,
  item-delimiter and sequence-delimiter tags are exactly the ones godicom's own
  encapsulation and sequence code is built from, and exactly the ones a
  `JSONKey` round-trip is most likely to hit. Now `ParseUint`, matching
  pydicom's `int(arg, 16)`, which has no width at all. The parenthesised form
  `(FFFE,E000)` always worked, because it parses two 16-bit halves
- godicom now builds on 32-bit platforms at all. `encaps.Encapsulate`'s Basic
  Offset Table overflow guard was written `total > (1<<32)-1`, which is exact in
  pydicom because Python integers are arbitrary-precision but overflows `int` at
  compile time on a 32-bit target

### Changed
- **BREAKING (internal-facing): the declared value length is carried as `uint32`
  rather than `int`** through the header, deferred-read and sequence paths. A
  DICOM length is unsigned 32 bits and `0xFFFFFFFF` is the undefined-length
  sentinel; in an `int` on a 32-bit platform that sentinel is `-1`, so the code
  would have compiled and then quietly misread every undefined-length sequence
  and encapsulated pixel-data element. No exported signature changes, and two
  `uint32(length)` casts at call sites went away
- `golibjpeg` and `goopenjpeg` are now v1.3.0, adding prebuilt libraries for
  `darwin/amd64` and `windows/arm64` — six platforms each, up from four. Both
  now also build everywhere else instead of failing to compile, and return an
  error wrapping `ErrUnsupportedPlatform` from every entry point rather than
  panicking, so importing godicom is safe on any platform Go targets
- Docs: the README documents platform support and binary size. Each binary
  embeds one platform's codec libraries, never all twelve: a `linux/amd64`
  `cmd/godicom` build is 11.0 MB, where embedding every library unconditionally
  would make it 24.3 MB
- Docs: `CompressPixelData` listed its supported targets without HTJ2K, which
  `pixels.EncodeFrame` has dispatched on for some time, and spelled JPEG-LS as
  though it were one transfer syntax
- Docs: README covers the two diagnostics v0.28.0 added — `WriteOptions.OnDiagnostic`
  was not mentioned at all, and the read section predated both the VR
  disagreement kind and `Diagnostic.Path` naming the sequence item
- Docs: the README's snippets now exist as `Example` functions, so `go test`
  compiles them and checks their output, and pkg.go.dev renders them. Written
  the way a reader can run them: no fixture path, no filesystem — each builds
  its own Part 10 bytes in memory. The write-diagnostic snippet is corrected
  along the way; it showed the diagnostic's own message where the caller
  actually gets it wrapped in `error writing dataset`

### Added
- CI cross-builds and vets `windows/386`, `linux/386`, `linux/arm` (including
  `GOARM=5`), `linux/riscv64`, `linux/ppc64le`, `js/wasm` and `wasip1/wasm`, and
  runs the whole test suite on `linux/386`. Compiling is not enough to catch a
  width mistake — the repeater-mask bug above compiled fine and only showed when
  run. `linux/mips` and `linux/mipsle` are excluded because purego does not
  build for them yet
- Tests that need a native codec now skip, rather than fail, on a platform with
  no prebuilt library, so a real failure is visible among them

## [0.28.0] - 2026-08-24

### Changed
- **BREAKING: `Diagnostic.Path` is now `[]PathStep` instead of `[]Tag`**, so it
  names which *item* of each enclosing sequence the anomaly came from and not
  merely which sequence. PS3.5 gives sequence items an ordinal position and
  nothing else to identify them by, so a forty-item sequence used to produce
  forty diagnostics that read identically. `PathStep{Tag, Item}` renders the way
  DICOM tooling spells it — `(0040,0100)[0]` — and a whole path reads
  `in (0008,1140)[1] > (0008,1110)[1]` in `Diagnostic.Error()` and in the
  `sequence_path` log attribute. `Item` is -1, and the subscript is dropped, when
  the sequence was entered but no item was: the item header itself was
  unreadable. Callers comparing `d.Path[0]` to a `Tag` compare it to
  `PathStep{Tag: t, Item: i}` instead, or read `d.Path[0].Tag`
- Internal: the read and write chains carry the encoding triple
  (`isImplicitVR`, `isLittleEndian`, `charsets`) as one `codecContext` instead of
  three parameters threaded through fourteen signatures. The two bools are
  adjacent and interchangeable, so a transposed call site used to compile and
  silently encode or parse a file in the wrong byte order

### Added
- **VR disagreement diagnostics**: reading now reports an explicit VR the data
  dictionary cannot reconcile with its tag — a `(0010,0010)` encoded as `SH`
  when the dictionary says `PN`, or a known tag sent as `UN`. The new
  `DiagnosticVRMismatch` kind carries the encoded VR in `Diagnostic.VR` and the
  dictionary's in the new `Diagnostic.ExpectedVR` field. The parse is
  unchanged — godicom still keeps the VR the file gave it, because what the file
  says is what the file means — so this is pure information about
  interoperability, and returning the diagnostic from the hook turns it into a
  read failure. Private tags, tags absent from the dictionary, and implicit VR
  are excluded: they have no dictionary VR to fall short of. A dictionary entry
  naming more than one permitted VR (`"OB or OW"`, `"US or SS"`) is satisfied by
  any of them, so `PixelData` does not report on every image. The check costs a
  dictionary lookup per element, so it is skipped unless an `OnDiagnostic` hook
  is set or warn-level logging is on
- **Write diagnostics**: `WriteOptions.OnDiagnostic func(Diagnostic) error`
  mirrors `ReadOptions.OnDiagnostic` and reports values the writer would
  otherwise encode silently even though godicom's own reader raises a
  diagnostic on the result — a fractional `float64` in an `IS` (`1.5` is not an
  integer string), a `DS` longer than the 16 bytes PS3.5 allows, an `IS`
  outside `[-2^31, 2^31)`. The new `DiagnosticInvalidValue` kind identifies
  them; they carry no `Offset`, since nothing was read. Returning `nil` keeps
  the old behaviour and writes the value as it stands, so no existing caller
  changes; returning the diagnostic fails the write:

  ```go
  opts := &WriteOptions{OnDiagnostic: func(d Diagnostic) error { return d }}
  ```

  This is the three-way choice pydicom spells `IGNORE` / `WARN` / `RAISE` in
  `config.settings.writing_validation_mode`, without a mode enum: whether the
  hook is set, and what it returns, says which one the caller wants. Values
  written back from the bytes they were read as are not offered — they are not
  re-encoded, and the read had its own chance to report them

### Fixed
- A `DS` element holding a plain `float64` — what `SetFloat` / `SetFloats`
  store, since `DS` is a float VR — was written with an unbounded `%g` while
  PS3.5 caps `DS` at 16 bytes. `SetFloat(SliceThickness, 1.0/3.0)` wrote
  `"0.3333333333333333"` (18 bytes), which godicom's own `IsValidDS` rejects
  and a strict receiver may refuse. The writer now applies the same
  `FormatNumberAsDS` truncation the `DS` type and pydicom's
  `format_number_as_ds` use, so a value stored as a `float64` reaches the file
  identically to the same value stored as a `DS`. A `DS` parsed from a file
  still round-trips its original string byte for byte, over-long or not
- `NaN` and the infinities were written into a `DS` as the literal bytes
  `"NaN"` / `"+Inf"` / `"-Inf"` with no error reported anywhere, and godicom's
  tolerant `ParseDS` read them back — a decimal string has no spelling for any
  of them. `SetFloat` / `SetFloats` now reject them for a `DS` tag at the call
  site, and the writer refuses them rather than emitting an invalid `DS` or
  `IS`. `FD` and `FL` are unaffected: they represent all three exactly, per
  IEEE 754

## [0.27.0] - 2026-08-20

### Added
- **Diagnostics**: `ReadOptions.OnDiagnostic` reports parse anomalies — a value
  shorter than its length field, a header cut off mid-element, a sequence item
  header past the end of the file, a deferred value whose source has gone away —
  each carrying its tag, VR, byte offset, and enclosing sequences. `Diagnostic`
  is itself an `error`, so returning it from the hook rejects the file while
  returning `nil` keeps the tolerant default
- **Setters**: dictionary-VR setters `SetString` / `SetInt` / `SetFloat` /
  `SetBytes` / `SetSequence`, the plural `SetStrings` / `SetInts` / `SetFloats`,
  and `SetDA` / `SetTM` / `SetDT` / `SetPN` / `SetDS` / `SetIS`. Each resolves
  the VR from the data dictionary and rejects value kinds that VR cannot hold.
  Tags outside the dictionary still need
  `Set(NewDataElement(tag, vr, value))`

### Changed
- **BREAKING**: transfer syntax parameters and returns are `uid.UID`, not
  `string`: `EncodeDataset`, `WriteDataset`, `Dataset.Encode`, `DecodeDataset`,
  `DecodeDatasetContext`, `CompressPixelData`, `CompressPixelDataContext`,
  `FileDataset.TransferSyntaxUID`, and `pixels.FileSource`. String literals
  still compile at call sites; `string` variables need an explicit `uid.UID(...)`
- `Read` hands a reader that already offers `io.ReaderAt` plus `Size() int64`
  (`*bytes.Reader`, `*strings.Reader`, `*io.SectionReader`) to the parser as-is
  instead of wrapping it in Seek+Read, so neither the parse nor a later deferred
  load moves the caller's offset
- Internal: one shared data element header decoder for the streaming and
  in-memory readers; the sequence readers return errors instead of latching
  them on the read context

### Fixed
- Deferred values were permanently unreachable after `Read` from a seekable
  reader that is not an `*os.File` (`bytes.Reader`, `io.SectionReader`,
  range-request adapters): the tag stayed listed while `Get` / `GetBytes`
  reported it absent
- Deferred values in a Deflated dataset read through `ReadBytes` / `Read`
  loaded from the still-compressed buffer, failing with a tag or VR mismatch.
  `ReadFile` was unaffected
- The streaming reader silently dropped a defined-length sequence whose length
  ran past the end of the file, complete items included. It now parses the items
  that are present and reports a truncated item, matching `ReadBytes` and
  pydicom's `read_sequence_item`
- A diagnostic raised inside a sequence — and a hook error rejecting it — now
  reaches the caller instead of being swallowed by the sequence readers
- `AT` elements were encoded as a single 32-bit value instead of the group /
  element pair of 16-bit values required by PS3.5 7.1.1. The two coincide under
  big endian and differ under little endian, so godicom wrote `AT` values that
  it and every other implementation read back swapped: `(0018,1063)` came back
  as `(1063,0018)`. `encodeAT` also accepts `[]Tag`, `[]int` and
  `*MultiValue[int]` now, so `SetInts` on an `AT` tag encodes rather than
  silently producing an empty value

## [0.26.0] - 2026-08-13

### Added
- **Logging**: quiet-by-default `log/slog` foundation (`WithLogger` /
  `LoggerFromContext` / `SetDefaultLogger`); `ReadOptions.Logger` /
  `WriteOptions.Logger`; `*Context` entry points; CLI `godicom show -debug`
- **Pixels**: `PixelArray` and `DisplayFrame` (pydicom-style pixel access +
  8-bit display pipeline)
- **Pixels**: HTJ2K encode for transfer syntaxes `.201` / `.202` / `.203` via
  [goopenjpeg](https://github.com/godicom-dev/goopenjpeg) v1.2.0

### Changed
- Docs: README aligned with pydicom-style layout (badges, description, examples)

## [0.25.1] - 2026-08-07

### Fixed
- CI: bump `golibjpeg` to v1.2.1 (Linux/Windows prebuilt libs include `golibjpeg_encode`; v1.2.0 native artifacts were incomplete)
- `gofmt` on `pixels/encode.go`

## [0.25.0] - 2026-08-07

### Added
- **Pixels**: JPEG baseline / extended / lossless and JPEG-LS encode via `golibjpeg` v1.2.0 (`EncodeFrame`, `CompressPixelData`)

### Fixed
- **Read**: ambiguous VR elements (`OB or OW`, `US or OW`, `US or SS`, `US or SS or OW`) keep raw `[]byte` at read time so Implicit VR LE `PixelData` works with `GetBytes` / `PixelBytes` (fixes #45)

### Changed
- **golibjpeg** dependency: v1.1.2 → v1.2.0

**Tests**: 731 passed

## [0.24.0] - 2026-07-18

### Added
- **uid**: `GenerateUID` / `MustGenerateUID` with `WithPrefix`, `WithUUIDPrefix`, `WithEntropy` (pydicom `generate_uid` behaviour; default root `1.2.826.0.1.3680043.8.498.`)
- **Read**: `Read(io.Reader)` — seekable sources (`*os.File`, `io.ReadSeeker`) parse without `ReadAll`; `StopBeforePixels` / `DeferSize` / `SpecificTags` skip large values; deferred loads reopen the file path

### Changed
- **ReadFile** uses the seekable streaming path (no full-file `ReadAll`)
- Docs: English product-first README / TODO; add [PARITY.md](PARITY.md) coverage map

## [0.23.0] - 2026-07-13

### Added
- **FileDataset**: `EncodeFile` / `Write` / `ReadBytes` — encode/decode Part 10 DICOM files to/from memory (DICOMweb STOW/WADO payloads)
- **dicomjson**: `ParseDatasets` / `MarshalDatasets` — JSON array of datasets (QIDO-RS / WADO-RS metadata)

## [0.22.1] - 2026-07-13

### Fixed
- **Write/Encode**: remove package-level write state (`sqDepth` / `visitingSequences`) so concurrent `EncodeDataset` / `SaveAs` is race-free

## [0.22.0] - 2026-07-13

### Added
- **Dataset**: `DecodeDataset` / `DecodeDatasetEncoding` — decode dataset bytes without preamble / File Meta (DIMSE Identifiers / C-STORE payloads; Deflated TS supported)

**Tests**: 696 passed (v0.21.0: 692); roundtrip + pynetdicom C-FIND Identifier golden

## [0.21.0] - 2026-07-13

### Added
- **Dataset**: `Encode` / `EncodeEncoding` and package `EncodeDataset` / `EncodeDatasetEncoding` / `WriteDataset` — encode a dataset without preamble or File Meta (DIMSE C-STORE / C-FIND payloads; Deflated TS supported)

**Tests**: 692 passed (v0.20.0: 686); EncodeDataset Implicit VR LE matches pynetdicom `c_store_ds` golden bytes

## [0.20.0] - 2026-07-13

### Added
- **Pixels / FileDataset**: JPEG 2000 Lossless (and default lossy JPEG 2000) via `goopenjpeg` v1.1.1 `Encode`
- `CompressPixelData` now supports `JPEG2000Lossless` / `JPEG2000` in addition to RLE / Deflated / native

### Notes
- JPEG / JPEG-LS **encode** remains unavailable (`golibjpeg` / pylibjpeg-libjpeg have no encoder)

**Tests**: 686 passed (v0.19.0: 685)

## [0.19.0] - 2026-07-13

### Added
- **Encaps**: `Encapsulate` / `EncapsulateExtended` / `FragmentFrame` / `ItemizeFragment` (pydicom encaps write path)
- **Pixels**: `EncodeFrame` / `EncodeFrames` for native, RLE Lossless, Deflated Image Frame Compression
- **FileDataset**: `CompressPixelData` / `SetEncodedPixelData` to re-encode Pixel Data and update Transfer Syntax

### Notes
- JPEG / JPEG-LS / JPEG2000 **encode** still requires golibjpeg / goopenjpeg encode APIs (decode-only for now)

**Tests**: 685 passed (v0.18.0: 676)

## [0.18.0] - 2026-07-13

### Added
- **Pixels**: `UnpackSamples`, `ApplyRescale` / `ApplyModalityLUT`, `ApplyWindowing` / `ApplyVOI` / `ApplyVOILUT`, `InvertValues` / `ApplyPresentationLUTShape`
- **FileDataset**: `PixelSamples`, `ApplyModalityLUT`, `ApplyVOILUT`, `ApplyPresentationLUTShape` (pydicom display pipeline helpers; not auto-applied in `PixelBytes`)
- **Dataset**: `GetFloats` for multi-valued DS/FD window tags

**Tests**: 676 passed (v0.17.0: 667)

## [0.17.0] - 2026-07-13

### Added
- **Pixels**: `ConvertColorSpace` (YBR_FULL ↔ RGB), `ExpandYBR422`, planar↔interleaved helpers
- **Pixels**: when `WithRaw(false)` (default), apply YBR→RGB and PlanarConfiguration normalization after decode

**Tests**: 667 passed (v0.16.0: 662)

## [0.16.0] - 2026-07-13

### Added
- **Dataset**: `FormattedLines` / `Top`; nested `String()` pretty-print for sequences (pydicom `formatted_lines` / `top`)
- **Element**: UI string values resolve to known UID names in `ReprValue`

**Tests**: 662 passed (v0.15.0: 656)

## [0.15.0] - 2026-07-13

### Added
- **ValueRep**: `IsValidDS` / `IsValidIS` / `ISInRange`; `FormatNumberAsDS` / `DSFromFloat` (pydicom `format_number_as_ds`)
- **ValueRep**: `DA`/`TM`/`DT`/`DS`/`IS`/`PersonName` `Equal` helpers
- **PersonName**: `PersonNameParts` / `PersonNameFromParts` / `PersonNameFromVeterinary` (named components + veterinary)

**Tests**: 656 passed (v0.14.0: 645)

## [0.14.0] - 2026-07-13

### Added
- **Dataset**: pydicom-aligned APIs — `Clear`, `Pop`, `Update`, `GroupDataset`, `RemovePrivateTags`, `ElementByKeyword`, `IterAll`, `Equal`
- **Dataset**: `IsOriginalEncoding` / `SetOriginalEncoding` / `SetWriteEncoding`; deep `Element.Equal` for sequences

**Tests**: 645 passed (v0.13.0: 633)

## [0.13.0] - 2026-07-13

### Added
- **Charset**: pydicom-aligned write tests — changed SpecificCharacterSet re-encode, WritePN/WriteText multi-charset, Japanese ISO-2022 PN encode

### Fixed
- **Charset / File Writer**: changing `SpecificCharacterSet` forces text/PN re-encode from decoded Unicode (no longer writes stale `RawValue`)
- **Charset**: `EncodeString` rejects non-ASCII for default/ISO_IR 6 so ISO-2022 extensions are used
- **Charset**: multi-charset encode splits on `^`/`=`/`\` so escapes survive delimiter repertoire resets

### Removed
- Known limitation: Japanese ISO-2022 multi-byte encode (now covered for PN roundtrip)

**Tests**: 633 passed (v0.12.0: 623)

## [0.12.0] - 2026-07-13

### Added
- **File Writer**: implicit VR byte-layout tests for OD/OL/UC/UR/UN; UC multi-value explicit cases; file meta unchanged on non-enforced save
- **Reader / File Writer**: private-tag VR resolution from private dictionary during implicit reads; implicit↔explicit conversion keeps private creator/data VRs

### Fixed
- **Reader**: private creator tags resolve to LO; private data tags use creator + private dictionary instead of always UN on implicit VR read

**Tests**: 623 passed (v0.11.0: 615)

## [0.11.0] - 2026-07-13

### Added
- **File Writer**: pydicom-aligned encoding tests — EnforceFileFormat file-meta fill/SOP sync, private/invalid transfer syntax, mismatch checks, command-set reject, unknown VR, implicit↔explicit VR conversion

### Fixed
- **File Writer**: `determineWriteEncoding` rejects non-transfer-syntax UIDs, requires encoding args for private TS, and checks ImplicitVR/LittleEndian mismatches independently
- **File Writer**: reject Command Set group `(0000,eeee)` elements on file write; reject unknown VRs when encoding elements

**Tests**: 615 passed (v0.10.0: 603)

## [0.10.0] - 2026-07-10

### Added
- **File Writer**: reject group 2 elements in dataset; implicit VR + big endian validation; explicit/implicit SQ ambiguous VR via tag-number access
- **File Writer**: `writeFileMetaInfo` validates non-group-2 elements; raw ambiguous element conversion tests

### Fixed
- **File Writer**: ambiguous US/SS/LUTData correction converts `RawValue`-only payloads; non-ambiguous overlay OB no longer forced to OW

**Tests**: 603 passed (v0.9.0: 591)

## [0.9.0] - 2026-07-10

### Added
- **File Writer**: non-standard file-meta-only write (no preamble), preamble+file-meta roundtrip, dataset unchanged on save
- **File Writer**: `writeFileMetaInfo` non-standard tests (missing TransferSyntaxUID, progressive missing elements)
- **File Writer**: byte-identical roundtrip for `chrH31.dcm` and `chrFrenMulti.dcm`

**Tests**: 591 passed (v0.8.0: 583)

## [0.8.0] - 2026-07-10

### Added
- **File Writer**: pydicom-aligned ambiguous VR tests for LUTDescriptor VM3, LUTData, PixelData, and implicit sequence pixel-representation nearest lookup
- **File Writer**: non-standard write paths without enforced file format (no preamble, empty file meta, dataset-only)

### Fixed
- **File Writer**: `lutDescriptorFirstValue` handles `MultiValue[uint64]` / `MultiValue[int64]` so LUTData resolves to US when descriptor first value is 1

**Tests**: 583 passed (v0.7.0: 571)

## [0.7.0] - 2026-07-06

### Added
- **Charset**: fetch all 17 pydicom `FILE_PATIENT_NAMES` charset fixtures via `scripts/fetch-testdata.sh`
- **Charset**: read, write roundtrip, and byte-identical tests for Arabic, German, Hebrew, Korean (chrI2), GB18030 (chrX2), Japanese multi-charset PN (chrH31/H32), and related fixtures
- **Charset**: `chrFrenMulti` multi-valued PN/LO tests; `chrSQEncoding1` sequence charset inheritance test

### Fixed
- **Charset**: ISO-2022 IR 149 (Korean) decode strips `\x1b$)C` escapes before EUC-KR payload decode

### Known limitations
- Japanese ISO-2022 multi-byte **encode** not fully covered (read works, e.g. `chrSQEncoding.dcm`)
- `chrJapMulti` / `chrKoreanMulti` write roundtrip is value-identical but not byte-identical (SpecificCharacterSet padding differs)

**Tests**: 571 passed (v0.6.0: 531)

## [0.6.0] - 2026-07-04

### Added
- **File Writer**: byte-identical roundtrip tests for undefined-length sequences, private nested SQ, `MR_small_implicit.dcm`, and additional pydicom fixtures
- **File Writer**: implicit nested ambiguous VR via sequence owner/parent ancestor chain; pydicom-aligned P2 tests (index/nearest access, parent change, `(FFFF,FFFF)`, no-preamble/prefix)
- **File Writer**: `CorrectAmbiguousVRPreservingRaw` for implicit writes with in-memory values while keeping raw byte roundtrips
- **Reader**: undefined-length UN values parsed as sequences (PS3.5 6.2.2); raw byte preservation for other undefined-length elements
- **Reader**: `Force` read of small files without preamble; propagate transfer syntax encoding into nested sequence items
- Transfer syntax encoding from file meta; TS conversion roundtrip tests (BE↔LE); lazy ambiguous VR correction on `Get` for file-read elements
- `UN_sequence.dcm` semantic read test; overlay/waveform ambiguous VR tests; file meta validation/group length tests

### Fixed
- **File Writer**: nested sequences with repeated tags no longer skip items due to incorrect cycle detection
- **File Writer**: re-encode from typed values when output transfer syntax differs from read encoding (instead of writing stale RawValue bytes)
- **File Writer**: `encodeInts` handles `[]int` / `[]int64` (fixes empty LUTDescriptor on implicit write)
- **Ambiguous VR**: `WaveformBitsAllocated` tag lookup (was wrongly using `WaveformBitsStored`)

**Tests**: 531 passed (v0.5.0: 503)

## [0.5.0] - 2026-07-04

### Added
- **Charset**: ISO-2022 decode on read and encode on write for PN / LO / LT / SH / ST / UT / UC
- **Charset**: multi-charset code extensions, SQ item charset inheritance
- **Charset**: integration tests on pydicom `charset_files` (Latin, Greek, Russian, UTF-8, Japanese SQ)
- **Charset**: UTF-8 (`ISO_IR 192`) write/read roundtrip (`chrX1.dcm`)
- **PersonName**: component helpers (`FamilyName`, `GivenName`, `Formatted`, …) and `Dataset.GetPN`
- **Encaps**: `CountFragments` and expanded unit / real-DICOM integration tests
- **Docs**: `CHANGELOG.md`

### Changed
- Nested sequence ambiguous VR handling and `convertInts` odd-length validation

### Known limitations
- Japanese ISO-2022 multi-byte **encode** not fully covered (read works, e.g. `chrSQEncoding.dcm`)
- Broader `chr*.dcm` matrix not exhaustive

**Tests**: 503 passed (v0.4.0: 429)

## [0.4.0] - 2026-07-03

### Added
- **Value Representation**: `DA` / `TM` / `DT` types and `GetDA` / `GetTM` / `GetDT`
- **Value Representation**: `DS` / `IS` typed values and `GetDS` / `GetIS` (with `Original` string preserved)
- **CLI**: `godicom show` / `read` with file meta + dataset display
- **CLI**: `-t` / `--top` tag filter and `--no-meta` flag
- **File Writer**: pydicom `test_filewriter.py` subset — empty AT/LO, DA/TM/DT single/multi-value, empty sequence
- **File Writer**: ambiguous VR resolution on explicit write; nested SQ ambiguous VR tests
- **Reader**: nested SQ ambiguous VR read fixes

### Fixed
- `convertInts` rejects odd-length byte values

**Tests**: 429 passed (v0.3.1: 389)

## [0.3.1] - 2026-07-02

### Fixed
- Metadata roundtrip gaps from v0.3.0 follow-up

## [0.3.0] - 2026-07-02

### Added
- File writer metadata roundtrip improvements
- `Dataset.Walk` and `Dataset.Clone`

## [0.2.0] - 2026-07-02

### Added
- Pixel read path: `PixelBytes` / `PixelFrames`, encaps frame splitting
- JPEG / JPEG-LS / JPEG 2000 / RLE decode via `golibjpeg`, `goopenjpeg`, `gorle`
- DICOM JSON Model (`dicomjson` subpackage)
- Deferred read (`DeferSize`), encapsulated pixel data read
- Private dictionary query API and code-generated UID dictionary

## [0.1.0] - 2026-07-02

### Added
- Initial release: DICOM file read/write, tag dictionary, basic VR conversion
- pydicom test file compatibility for core read paths

[Unreleased]: https://github.com/godicom-dev/godicom/compare/v0.37.0...HEAD
[0.37.0]: https://github.com/godicom-dev/godicom/compare/v0.36.0...v0.37.0
[0.36.0]: https://github.com/godicom-dev/godicom/compare/v0.35.0...v0.36.0
[0.35.0]: https://github.com/godicom-dev/godicom/compare/v0.34.0...v0.35.0
[0.34.0]: https://github.com/godicom-dev/godicom/compare/v0.33.0...v0.34.0
[0.33.0]: https://github.com/godicom-dev/godicom/compare/v0.32.0...v0.33.0
[0.32.0]: https://github.com/godicom-dev/godicom/compare/v0.31.0...v0.32.0
[0.31.0]: https://github.com/godicom-dev/godicom/compare/v0.30.0...v0.31.0
[0.30.0]: https://github.com/godicom-dev/godicom/compare/v0.29.0...v0.30.0
[0.29.0]: https://github.com/godicom-dev/godicom/compare/v0.28.0...v0.29.0
[0.28.0]: https://github.com/godicom-dev/godicom/compare/v0.27.0...v0.28.0
[0.27.0]: https://github.com/godicom-dev/godicom/compare/v0.26.0...v0.27.0
[0.26.0]: https://github.com/godicom-dev/godicom/compare/v0.25.1...v0.26.0
[0.25.1]: https://github.com/godicom-dev/godicom/compare/v0.25.0...v0.25.1
[0.25.0]: https://github.com/godicom-dev/godicom/compare/v0.24.0...v0.25.0
[0.24.0]: https://github.com/godicom-dev/godicom/compare/v0.23.0...v0.24.0
[0.23.0]: https://github.com/godicom-dev/godicom/compare/v0.22.1...v0.23.0
[0.22.1]: https://github.com/godicom-dev/godicom/compare/v0.22.0...v0.22.1
[0.22.0]: https://github.com/godicom-dev/godicom/compare/v0.21.0...v0.22.0
[0.21.0]: https://github.com/godicom-dev/godicom/compare/v0.20.0...v0.21.0
[0.20.0]: https://github.com/godicom-dev/godicom/compare/v0.19.0...v0.20.0
[0.19.0]: https://github.com/godicom-dev/godicom/compare/v0.18.0...v0.19.0
[0.18.0]: https://github.com/godicom-dev/godicom/compare/v0.17.0...v0.18.0
[0.17.0]: https://github.com/godicom-dev/godicom/compare/v0.16.0...v0.17.0
[0.16.0]: https://github.com/godicom-dev/godicom/compare/v0.15.0...v0.16.0
[0.15.0]: https://github.com/godicom-dev/godicom/compare/v0.14.0...v0.15.0
[0.14.0]: https://github.com/godicom-dev/godicom/compare/v0.13.0...v0.14.0
[0.13.0]: https://github.com/godicom-dev/godicom/compare/v0.12.0...v0.13.0
[0.12.0]: https://github.com/godicom-dev/godicom/compare/v0.11.0...v0.12.0
[0.11.0]: https://github.com/godicom-dev/godicom/compare/v0.10.0...v0.11.0
[0.10.0]: https://github.com/godicom-dev/godicom/compare/v0.9.0...v0.10.0
[0.9.0]: https://github.com/godicom-dev/godicom/compare/v0.8.0...v0.9.0
[0.8.0]: https://github.com/godicom-dev/godicom/compare/v0.7.0...v0.8.0
[0.7.0]: https://github.com/godicom-dev/godicom/compare/v0.6.0...v0.7.0
[0.6.0]: https://github.com/godicom-dev/godicom/compare/v0.5.0...v0.6.0
[0.5.0]: https://github.com/godicom-dev/godicom/compare/v0.4.0...v0.5.0
[0.4.0]: https://github.com/godicom-dev/godicom/compare/v0.3.1...v0.4.0
[0.3.1]: https://github.com/godicom-dev/godicom/compare/v0.3.0...v0.3.1
[0.3.0]: https://github.com/godicom-dev/godicom/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/godicom-dev/godicom/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/godicom-dev/godicom/releases/tag/v0.1.0
