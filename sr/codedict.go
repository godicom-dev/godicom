package sr

import (
	_ "embed"
	"errors"
	"fmt"
	"maps"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"
)

// codesSource is the terminology, one fact per line. generate_sr_dict.py
// writes it from pydicom's sr/_concepts_dict.py, sr/_cid_dict.py and
// sr/_snomed_dict.py; the record types are described at the top of the file.
//
//go:embed codes.txt
var codesSource string

// ErrNoMatchingCode is returned when a keyword is not in the collection it was
// looked up in.
var ErrNoMatchingCode = errors.New("godicom: no matching code")

// ErrAmbiguousCode is returned when a keyword resolves to more than one code,
// which happens in a CID that reaches a word through two schemes or, within one
// scheme, through two codes.
var ErrAmbiguousCode = errors.New("godicom: ambiguous code")

// ErrNoSuchCollection is returned for a scheme designator or CID that PS3.16
// does not define.
var ErrNoSuchCollection = errors.New("godicom: no such code collection")

// concept is one code of one keyword, with the CIDs it belongs to.
type concept struct {
	code Code
	cids []int
}

// table is everything codes.txt holds.
type table struct {
	schemes     []string                        // in the order pydicom declares them
	concepts    map[string]map[string][]concept // scheme -> keyword -> codes
	cidNames    map[int]string                  // CID number -> the name PS3.16 gives it
	cidOrder    []int                           // in ascending order
	members     map[int]map[string][]string     // CID number -> scheme -> keywords
	memberOrder map[int][]string                // CID number -> the schemes it draws on, in file order
	srtToSCT    map[string]string               // a SNOMED code under the retired designator -> under SCT
}

var (
	loadOnce sync.Once
	loaded   *table
)

// data returns the terminology, reading codes.txt the first time it is asked
// for. A parse failure is a broken build rather than something a caller can act
// on -- the file is generated into the package and TestParseCodes keeps it
// readable -- so it panics instead of pushing the error through every caller.
func data() *table {
	loadOnce.Do(func() {
		t, err := parse(codesSource)
		if err != nil {
			panic("godicom/sr: " + err.Error())
		}
		loaded = t
	})

	return loaded
}

// parse reads the records codes.txt is made of.
func parse(source string) (*table, error) {
	t := &table{
		concepts:    map[string]map[string][]concept{},
		cidNames:    map[int]string{},
		members:     map[int]map[string][]string{},
		memberOrder: map[int][]string{},
		srtToSCT:    map[string]string{},
	}

	for line := range strings.SplitSeq(source, "\n") {
		if line == "" || line[0] == '#' {
			continue
		}
		if len(line) < 2 || line[1] != '\t' {
			return nil, fmt.Errorf("malformed record %q", clip(line))
		}

		kind, rest := line[0], line[2:]
		fields := strings.Split(rest, "\t")

		switch kind {
		case 'S':
			// scheme, keyword, code, cids, meaning
			if len(fields) != 5 {
				return nil, fmt.Errorf("record %q has %d fields, want 5", clip(line), len(fields))
			}
			scheme, keyword, value, cidList, meaning := fields[0], fields[1], fields[2], fields[3], fields[4]
			cids, err := parseCIDs(cidList)
			if err != nil {
				return nil, fmt.Errorf("record %q: %w", clip(line), err)
			}
			if _, seen := t.concepts[scheme]; !seen {
				t.schemes = append(t.schemes, scheme)
				t.concepts[scheme] = map[string][]concept{}
			}
			t.concepts[scheme][keyword] = append(t.concepts[scheme][keyword], concept{
				code: New(value, scheme, meaning),
				cids: cids,
			})
		case 'C':
			// cid, name
			if len(fields) != 2 {
				return nil, fmt.Errorf("record %q has %d fields, want 2", clip(line), len(fields))
			}
			number, err := strconv.Atoi(fields[0])
			if err != nil {
				return nil, fmt.Errorf("record %q: %w", clip(line), err)
			}
			t.cidNames[number] = fields[1]
			t.cidOrder = append(t.cidOrder, number)
			if _, seen := t.members[number]; !seen {
				t.members[number] = map[string][]string{}
			}
		case 'I':
			// cid, scheme, keyword
			if len(fields) != 3 {
				return nil, fmt.Errorf("record %q has %d fields, want 3", clip(line), len(fields))
			}
			number, err := strconv.Atoi(fields[0])
			if err != nil {
				return nil, fmt.Errorf("record %q: %w", clip(line), err)
			}
			scheme, keyword := fields[1], fields[2]
			if t.members[number] == nil {
				t.members[number] = map[string][]string{}
			}
			if _, seen := t.members[number][scheme]; !seen {
				t.memberOrder[number] = append(t.memberOrder[number], scheme)
			}
			t.members[number][scheme] = append(t.members[number][scheme], keyword)
		case 'M':
			// srt code, sct code
			if len(fields) != 2 {
				return nil, fmt.Errorf("record %q has %d fields, want 2", clip(line), len(fields))
			}
			t.srtToSCT[fields[0]] = fields[1]
		default:
			return nil, fmt.Errorf("unknown record type %q in %q", kind, clip(line))
		}
	}

	if len(t.schemes) == 0 {
		return nil, errors.New("no schemes in the table")
	}

	return t, nil
}

// parseCIDs reads the comma separated CID numbers of an S record.
func parseCIDs(list string) ([]int, error) {
	if list == "" {
		return nil, nil
	}
	parts := strings.Split(list, ",")
	cids := make([]int, len(parts))
	for i, part := range parts {
		cid, err := strconv.Atoi(part)
		if err != nil {
			return nil, fmt.Errorf("bad CID number %q: %w", part, err)
		}
		cids[i] = cid
	}

	return cids, nil
}

// clip shortens a line for an error message.
func clip(line string) string {
	if len(line) > 60 {
		return line[:60] + "..."
	}
	return line
}

// Collection is a coding scheme such as SCT, or one of the contexts PS3.16
// defines such as CID2.
//
// A Collection is read-only and safe for concurrent use.
type Collection struct {
	name string
	cid  int // 0 for a coding scheme

	once  sync.Once
	codes map[string]Code
}

// NewCollection returns the collection with the given name: a scheme
// designator such as "SCT", or a CID such as "CID2", in any case for the "CID"
// part.
func NewCollection(name string) (*Collection, error) {
	t := data()

	if key := collectionKey(name); strings.HasPrefix(key, "CID") {
		number, err := strconv.Atoi(key[3:])
		if err != nil {
			return nil, fmt.Errorf("%w: %q is not a CID name", ErrNoSuchCollection, name)
		}
		if _, ok := t.cidNames[number]; !ok {
			return nil, fmt.Errorf("%w: %q", ErrNoSuchCollection, name)
		}
		return &Collection{name: key, cid: number}, nil
	}

	if _, ok := t.concepts[name]; !ok {
		return nil, fmt.Errorf("%w: %q", ErrNoSuchCollection, name)
	}

	return &Collection{name: name}, nil
}

// Scheme returns the coding scheme with the given designator, such as "SCT".
func Scheme(designator string) (*Collection, error) {
	return NewCollection(designator)
}

// CID returns the context with the given number, such as 2 for CID2.
func CID(number int) (*Collection, error) {
	if _, ok := data().cidNames[number]; !ok {
		return nil, fmt.Errorf("%w: CID%d", ErrNoSuchCollection, number)
	}

	return &Collection{name: "CID" + strconv.Itoa(number), cid: number}, nil
}

// collectionKey normalises the designator a CID may be named with: pydicom
// accepts "CID2" and "cid2" alike and keeps the digits as given.
func collectionKey(name string) string {
	if len(name) >= 3 && strings.EqualFold(name[:3], "CID") {
		return "CID" + name[3:]
	}

	return name
}

// Name returns the collection's name: the scheme designator, or "CID" and the
// number of the context.
func (c *Collection) Name() string { return c.name }

// SchemeDesignator returns the coding scheme the collection's codes come from,
// which is its name for a scheme and "CID" plus a number for a context.
func (c *Collection) SchemeDesignator() string { return c.name }

// IsCID reports whether the collection is one of the contexts of PS3.16 rather
// than a coding scheme.
func (c *Collection) IsCID() bool { return c.cid != 0 }

// Dir returns the keywords of the collection, sorted, or those of them that
// contain any of filters, ignoring case.
//
// pydicom's Collection.dir() is the same function; the keywords are the camel
// case forms of the code meanings that pydicom's attribute access uses, so
// Dir("thoracic") on CID4 returns the five anatomical entries whose keyword
// mentions it.
func (c *Collection) Dir(filters ...string) []string {
	t := data()

	var keywords []string
	if c.cid != 0 {
		seen := make(map[string]bool)
		for _, scheme := range t.memberOrder[c.cid] {
			for _, keyword := range t.members[c.cid][scheme] {
				if !seen[keyword] {
					seen[keyword] = true
					keywords = append(keywords, keyword)
				}
			}
		}
	} else {
		for keyword := range t.concepts[c.name] {
			keywords = append(keywords, keyword)
		}
	}

	if len(filters) > 0 {
		lowered := make([]string, len(filters))
		for i, filter := range filters {
			lowered[i] = strings.ToLower(filter)
		}
		kept := keywords[:0]
		for _, keyword := range keywords {
			lower := strings.ToLower(keyword)
			for _, filter := range lowered {
				if strings.Contains(lower, filter) {
					kept = append(kept, keyword)
					break
				}
			}
		}
		keywords = kept
	}

	sort.Strings(keywords)

	return keywords
}

// Lookup returns the code a keyword stands for, such as "Lung" in SCT.
//
// A context may reach a keyword through more than one scheme -- CID8134 has
// "ArcuateFasciculus" in both FMA and NEU -- and, in principle, through more
// than one code within a scheme; both are ErrAmbiguousCode rather than a
// silent choice. A keyword the collection does not have is ErrNoMatchingCode.
func (c *Collection) Lookup(keyword string) (Code, error) {
	return c.lookup(data(), keyword)
}

// lookup is Lookup against a given table, so a test can hand it a small one.
func (c *Collection) lookup(t *table, keyword string) (Code, error) {
	if c.cid != 0 {
		var schemes []string
		for _, scheme := range t.memberOrder[c.cid] {
			for _, member := range t.members[c.cid][scheme] {
				if member == keyword {
					schemes = append(schemes, scheme)
					break
				}
			}
		}

		switch len(schemes) {
		case 0:
			return Code{}, fmt.Errorf("%w for keyword %q in %s", ErrNoMatchingCode, keyword, c.name)
		case 1:
			return t.codeIn(c.cid, c.name, schemes[0], keyword)
		default:
			return Code{}, fmt.Errorf("%w for keyword %q in %s: %s all contain it",
				ErrAmbiguousCode, keyword, c.name, strings.Join(schemes, ", "))
		}
	}

	return t.codeIn(c.cid, c.name, c.name, keyword)
}

// codeIn returns the one code keyword has in the scheme, restricted to the CIDs
// the code belongs to when cid is not zero.
func (t *table) codeIn(cid int, name, scheme, keyword string) (Code, error) {
	entries := t.concepts[scheme][keyword]
	switch len(entries) {
	case 0:
		if cid != 0 {
			return Code{}, fmt.Errorf("%w for keyword %q in %s", ErrNoMatchingCode, keyword, name)
		}
		return Code{}, fmt.Errorf("%w for keyword %q in scheme %q", ErrNoMatchingCode, keyword, scheme)
	case 1:
		return entries[0].code, nil
	}

	var found []Code
	for _, entry := range entries {
		if cid == 0 || containsCID(entry.cids, cid) {
			found = append(found, entry.code)
		}
	}

	switch {
	case len(found) == 0:
		if cid != 0 {
			return Code{}, fmt.Errorf("%w for keyword %q in %s", ErrNoMatchingCode, keyword, name)
		}
		return Code{}, fmt.Errorf("%w for keyword %q in scheme %q", ErrNoMatchingCode, keyword, scheme)
	case len(found) == 1:
		return found[0], nil
	}

	values := make([]string, len(found))
	for i, code := range found {
		values[i] = code.Value
	}

	if cid != 0 {
		return Code{}, fmt.Errorf("%w for keyword %q in %s: %s",
			ErrAmbiguousCode, keyword, name, strings.Join(values, ", "))
	}

	return Code{}, fmt.Errorf("%w for keyword %q in scheme %q: %s",
		ErrAmbiguousCode, keyword, scheme, strings.Join(values, ", "))
}

// containsCID reports whether cids holds cid.
func containsCID(cids []int, cid int) bool {
	for _, held := range cids {
		if held == cid {
			return true
		}
	}

	return false
}

// Concepts returns the collection's keywords and their codes.
//
// A keyword that resolves to nothing is left out. With the terminology this
// package ships those are only the two of CID8134 that reach two schemes, which
// have no one code to return and which Lookup refuses; every other keyword of
// every collection is in the map, which TestEveryKeywordResolves checks over the
// whole table.
func (c *Collection) Concepts() map[string]Code {
	c.once.Do(func() {
		keywords := c.Dir()
		codes := make(map[string]Code, len(keywords))
		for _, keyword := range keywords {
			if code, err := c.Lookup(keyword); err == nil {
				codes[keyword] = code
			}
		}
		c.codes = codes
	})

	return c.codes
}

// ContainsKeyword reports whether the collection has the keyword.
//
// A keyword the collection lists may still be ambiguous in it -- CID8134 lists
// two words that two schemes both contain -- so this answers about membership,
// which is what Dir says, rather than about Lookup succeeding. pydicom's
// "keyword in collection" goes through the attribute lookup and raises there.
func (c *Collection) ContainsKeyword(keyword string) bool {
	t := data()

	if c.cid != 0 {
		for _, scheme := range t.memberOrder[c.cid] {
			for _, member := range t.members[c.cid][scheme] {
				if member == keyword {
					return true
				}
			}
		}
		return false
	}

	_, ok := t.concepts[c.name][keyword]

	return ok
}

// ContainsCode reports whether the collection holds a code that denotes the
// same concept as code, by the comparison Code.Equal uses.
func (c *Collection) ContainsCode(code Code) bool {
	for _, held := range c.Concepts() {
		if held.Equal(code) {
			return true
		}
	}

	return false
}

// String returns the collection as a table of its keywords and codes, formatted
// the way pydicom formats one, so the two can be compared line for line.
//
// A context with no keywords, of which PS3.16 defines twelve, has nothing to
// tabulate; pydicom raises there, this returns the headers alone.
func (c *Collection) String() string {
	concepts := c.Concepts()
	// A keyword that resolves to nothing has no row to print. With this
	// terminology that is only the two of CID8134 that reach two schemes; see
	// TestAmbiguousKeywordsInCID8134.
	keywords := slices.Sorted(maps.Keys(concepts))

	nameWidth, codeWidth, schemeWidth := 11, 6, 8
	for _, keyword := range keywords {
		code := concepts[keyword]
		nameWidth = max(nameWidth, utf8.RuneCountInString(keyword)+2)
		codeWidth = max(codeWidth, utf8.RuneCountInString(code.Value)+2)
		schemeWidth = max(schemeWidth, utf8.RuneCountInString(code.SchemeDesignator)+2)
	}

	var b strings.Builder
	if c.cid != 0 {
		b.WriteString(c.name)
		b.WriteByte('\n')
		writeRow(&b, nameWidth, "Attribute", codeWidth, "Code", schemeWidth, "Scheme", "Meaning")
		writeRow(&b, nameWidth, "---------", codeWidth, "----", schemeWidth, "------", "-------")
		for _, keyword := range keywords {
			code := concepts[keyword]
			writeRow(&b, nameWidth, keyword, codeWidth, code.Value, schemeWidth, code.SchemeDesignator, code.Meaning)
		}
	} else {
		b.WriteString("Scheme: " + c.name)
		b.WriteByte('\n')
		writeRow(&b, nameWidth, "Attribute", codeWidth, "Code", 0, "", "Meaning")
		writeRow(&b, nameWidth, "---------", codeWidth, "----", 0, "", "-------")
		for _, keyword := range keywords {
			code := concepts[keyword]
			writeRow(&b, nameWidth, keyword, codeWidth, code.Value, 0, "", code.Meaning)
		}
	}

	return strings.TrimSuffix(b.String(), "\n")
}

// writeRow writes three padded columns followed by a free text one, the layout
// of both tables. A width of zero drops a column, which is how a scheme's table
// leaves out the scheme every row would repeat.
func writeRow(b *strings.Builder, nameWidth int, name string, codeWidth int, value string, schemeWidth int, scheme string, last string) {
	b.WriteString(pad(name, nameWidth))
	b.WriteByte(' ')
	b.WriteString(pad(value, codeWidth))
	b.WriteByte(' ')
	if schemeWidth > 0 {
		b.WriteString(pad(scheme, schemeWidth))
		b.WriteByte(' ')
	}
	b.WriteString(last)
	b.WriteByte('\n')
}

// pad left-aligns s in a field of width runes.
func pad(s string, width int) string {
	if n := utf8.RuneCountInString(s); n < width {
		return s + strings.Repeat(" ", width-n)
	}

	return s
}

// Concepts holds every collection: the coding schemes PS3.16 draws codes from
// and the contexts that group them.
type Concepts struct {
	order  []string
	byName map[string]*Collection
}

var (
	codesOnce sync.Once
	allCodes  *Concepts
)

// Codes returns every collection, reading the terminology on first call.
//
// It is the counterpart of pydicom's codes object:
//
//	codes := sr.Codes()
//	codes.Schemes()   // ["DCM", "SCT", ...]
//	codes.CIDs()      // ["CID2", "CID4", ...]
//	scheme, err := codes.Get("SCT")
func Codes() *Concepts {
	codesOnce.Do(func() {
		t := data()
		allCodes = &Concepts{byName: make(map[string]*Collection, len(t.schemes)+len(t.cidOrder))}

		for _, scheme := range t.schemes {
			allCodes.add(&Collection{name: scheme})
		}
		for _, cid := range t.cidOrder {
			allCodes.add(&Collection{name: "CID" + strconv.Itoa(cid), cid: cid})
		}
	})

	return allCodes
}

// add files a collection under its name.
func (c *Concepts) add(collection *Collection) {
	c.order = append(c.order, collection.name)
	c.byName[collection.name] = collection
}

// Collections returns the names of every collection: the schemes in the order
// the standard declares them, then the CIDs by number.
func (c *Concepts) Collections() []string {
	return append([]string(nil), c.order...)
}

// Schemes returns the designators of the coding schemes.
func (c *Concepts) Schemes() []string {
	var schemes []string
	for _, name := range c.order {
		if !strings.HasPrefix(name, "CID") {
			schemes = append(schemes, name)
		}
	}

	return schemes
}

// CIDs returns the names of the contexts, such as "CID2".
func (c *Concepts) CIDs() []string {
	var cids []string
	for _, name := range c.order {
		if strings.HasPrefix(name, "CID") {
			cids = append(cids, name)
		}
	}

	return cids
}

// Get returns the collection with the given name.
func (c *Concepts) Get(name string) (*Collection, error) {
	if collection, ok := c.byName[collectionKey(name)]; ok {
		return collection, nil
	}

	return nil, fmt.Errorf("%w: %q", ErrNoSuchCollection, name)
}
