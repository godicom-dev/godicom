package sr

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"testing"
)

// The expectations in this file come from pydicom 3.1.0.dev0, which is the
// checkout in pydicom/. Its tests/test_codes.py covers the same ground; the
// values below are what it asserts or what it produces.

func TestCodeEqual(t *testing.T) {
	mean := New("373098007", "SCT", "Mean Value of population")

	tests := []struct {
		name string
		a, b Code
		want bool
	}{
		{"the same code", mean, New("373098007", "SCT", "Mean Value of population"), true},
		{"a different meaning", mean, New("373098007", "SCT", "bla bla bla"), true},
		{"a different value", mean, New("373099004", "SCT", "Median Value of population"), false},
		{"a different scheme", mean, New("373098007", "DCM", "Mean Value of population"), false},
		{"the retired designator", mean, New("R-00317", "SRT", "Mean Value of population"), true},
		{"the retired designator, the other way", New("R-00317", "SRT", "Mean Value of population"), mean, true},
		{"a retired code with no SCT counterpart", mean, New("bla bal bla", "SRT", "Mean Value of population"), false},
		{"a scheme version", mean, mean.WithVersion("v1.0"), false},
	}

	for _, test := range tests {
		if got := test.a.Equal(test.b); got != test.want {
			t.Errorf("%s: %v.Equal(%v) = %v, want %v", test.name, test.a, test.b, got, test.want)
		}
		if got := test.b.Equal(test.a); got != test.want {
			t.Errorf("%s: equality is not symmetric: %v.Equal(%v) = %v", test.name, test.b, test.a, got)
		}
	}
}

func TestCodeEqualIgnoresVersionOfTheSameValue(t *testing.T) {
	a := New("39607008", "SCT", "Lung").WithVersion("2024c")
	b := New("39607008", "SCT", "Lung")
	if a.Equal(b) {
		t.Fatal("a code with a scheme version equals one without")
	}
	if !a.Equal(a.WithVersion("2024c")) {
		t.Fatal("a code does not equal itself with the same version")
	}
}

func TestCodeString(t *testing.T) {
	tests := []struct {
		code Code
		want string
	}{
		{New("39607008", "SCT", "Lung"), `(SCT, "39607008", "Lung")`},
		{New("39607008", "SCT", "Lung").WithVersion("2024c"), `(SCT, "39607008", "Lung", "2024c")`},
		{New(`a"b`, "TEST", `back\slash`), `(TEST, "a\"b", "back\\slash")`},
		{New("", "DCM", ""), `(DCM, "", "")`},
	}

	for _, test := range tests {
		if got := test.code.String(); got != test.want {
			t.Errorf("String() = %s, want %s", got, test.want)
		}
	}
}

func TestNewCollection(t *testing.T) {
	sct, err := Scheme("SCT")
	if err != nil {
		t.Fatalf("Scheme(SCT): %v", err)
	}
	if sct.Name() != "SCT" || sct.SchemeDesignator() != "SCT" || sct.IsCID() {
		t.Fatalf("SCT = %q/%q/%v", sct.Name(), sct.SchemeDesignator(), sct.IsCID())
	}

	cid, err := CID(2)
	if err != nil {
		t.Fatalf("CID(2): %v", err)
	}
	if cid.Name() != "CID2" || cid.SchemeDesignator() != "CID2" || !cid.IsCID() {
		t.Fatalf("CID2 = %q/%q/%v", cid.Name(), cid.SchemeDesignator(), cid.IsCID())
	}

	// The "CID" prefix is accepted in any case, as pydicom accepts it. The
	// digits are kept as written, which is what pydicom does too.
	lower, err := NewCollection("cid2")
	if err != nil {
		t.Fatalf("NewCollection(cid2): %v", err)
	}
	if lower.Name() != "CID2" {
		t.Fatalf("NewCollection(cid2).Name() = %q", lower.Name())
	}
}

func TestNewCollectionErrors(t *testing.T) {
	for _, name := range []string{"NOPE", "sct", "CID99999", "CIDabc", "CID"} {
		if _, err := NewCollection(name); !errors.Is(err, ErrNoSuchCollection) {
			t.Errorf("NewCollection(%q) error = %v, want ErrNoSuchCollection", name, err)
		}
	}
	if _, err := CID(99999); !errors.Is(err, ErrNoSuchCollection) {
		t.Errorf("CID(99999) error = %v, want ErrNoSuchCollection", err)
	}
}

func TestCollectionLookup(t *testing.T) {
	tests := []struct {
		collection string
		keyword    string
		want       Code
	}{
		{"SCT", "Lung", New("39607008", "SCT", "Lung")},
		{"SCT", "Right", New("24028007", "SCT", "Right")},
		{"UCUM", "Second", New("s", "UCUM", "second")},
		{"DCM", "Patient", New("121025", "DCM", "Patient")},
		// A keyword with a non-ASCII character in it, and the "^125^" one.
		{"DCM", "MüllerMethodPlanningForHipReplacement", New("112344", "DCM", "Müller Method Planning for Hip Replacement")},
		{"SCT", "_125Iodine", New("68630002", "SCT", "^125^Iodine")},
		{"CID2", "Transverse", New("62824007", "SCT", "Transverse")},
		{"CID606", "None_", New("260413007", "SCT", "None")},
		{"CID18", "_125Iodine", New("68630002", "SCT", "^125^Iodine")},
	}

	for _, test := range tests {
		collection, err := NewCollection(test.collection)
		if err != nil {
			t.Fatalf("%s: %v", test.collection, err)
		}
		got, err := collection.Lookup(test.keyword)
		if err != nil {
			t.Errorf("%s.Lookup(%q): %v", test.collection, test.keyword, err)
			continue
		}
		if !got.Equal(test.want) || got.Meaning != test.want.Meaning {
			t.Errorf("%s.Lookup(%q) = %v, want %v", test.collection, test.keyword, got, test.want)
		}
	}
}

func TestCollectionLookupErrors(t *testing.T) {
	scheme, _ := Scheme("UCUM")
	if _, err := scheme.Lookup("Foo"); !errors.Is(err, ErrNoMatchingCode) {
		t.Fatalf("Lookup(Foo) error = %v", err)
	} else if want := `for keyword "Foo" in scheme "UCUM"`; !strings.Contains(err.Error(), want) {
		t.Fatalf("error = %q, want it to contain %q", err, want)
	}

	cid, _ := CID(2)
	if _, err := cid.Lookup("Foo"); !errors.Is(err, ErrNoMatchingCode) {
		t.Fatalf("CID2.Lookup(Foo) error = %v", err)
	} else if want := `for keyword "Foo" in CID2`; !strings.Contains(err.Error(), want) {
		t.Fatalf("error = %q, want it to contain %q", err, want)
	}
}

// TestLookupAmbiguous builds the two tables that make a keyword ambiguous: one
// where a word reaches two codes of the same scheme, and one where it reaches
// two schemes. pydicom's terminology has neither, so there is nothing in
// codes.txt to look up -- its tests patch the dictionaries, and so does this.
func TestLookupAmbiguous(t *testing.T) {
	twoCodes := &table{
		schemes: []string{"TEST"},
		concepts: map[string]map[string][]concept{
			"TEST": {"Foo": {
				{code: New("BAR", "TEST", "Test A"), cids: []int{99999999999}},
				{code: New("BAZ", "TEST", "Test B"), cids: []int{99999999999}},
			}},
		},
		cidNames:    map[int]string{99999999999: "Test"},
		members:     map[int]map[string][]string{99999999999: {"TEST": {"Foo"}}},
		memberOrder: map[int][]string{99999999999: {"TEST"}},
	}

	scheme := &Collection{name: "TEST"}
	_, err := scheme.lookup(twoCodes, "Foo")
	if !errors.Is(err, ErrAmbiguousCode) {
		t.Fatalf("scheme lookup error = %v, want ErrAmbiguousCode", err)
	}
	if !strings.Contains(err.Error(), "BAR, BAZ") {
		t.Fatalf("error = %q, want both codes in it", err)
	}

	twoSchemes := &table{
		schemes: []string{"TEST", "TEST2"},
		concepts: map[string]map[string][]concept{
			"TEST":  {"Foo": {{code: New("BAR", "TEST", "Test A"), cids: []int{99999999999}}}},
			"TEST2": {"Foo": {{code: New("BAZ", "TEST2", "Test A"), cids: []int{99999999999}}}},
		},
		cidNames:    map[int]string{99999999999: "Test"},
		members:     map[int]map[string][]string{99999999999: {"TEST": {"Foo"}, "TEST2": {"Foo"}}},
		memberOrder: map[int][]string{99999999999: {"TEST", "TEST2"}},
	}

	cid := &Collection{name: "CID99999999999", cid: 99999999999}
	_, err = cid.lookup(twoSchemes, "Foo")
	if !errors.Is(err, ErrAmbiguousCode) {
		t.Fatalf("CID lookup error = %v, want ErrAmbiguousCode", err)
	}
	if !strings.Contains(err.Error(), "TEST, TEST2") {
		t.Fatalf("error = %q, want both schemes in it", err)
	}
}

func TestCollectionDir(t *testing.T) {
	cid4, _ := CID(4)

	want := []string{
		"CervicoThoracicSpine",
		"IntraThoracic",
		"StructureOfDescendingThoracicAorta",
		"ThoracicAorta",
		"ThoracicSpine",
	}
	for _, filter := range [][]string{{"Thoracic"}, {"thoracic"}, {"THORACIC"}, {"racic"}} {
		if got := cid4.Dir(filter...); strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("Dir(%v) = %v, want %v", filter, got, want)
		}
	}

	// Two filters widen the result rather than narrowing it: a keyword is kept
	// when it matches either.
	two := cid4.Dir("Thoracic", "aorta")
	for _, keyword := range append(want, "Aorta", "AscendingAorta", "AbdominalAorta", "PrimitiveAorta") {
		if !contains(two, keyword) {
			t.Errorf("Dir(Thoracic, aorta) = %v, want it to hold %s", two, keyword)
		}
	}

	if got := cid4.Dir("zzz"); len(got) != 0 {
		t.Errorf("Dir(zzz) = %v, want nothing", got)
	}

	// Sorted and unique whatever the table's order is.
	sct, _ := Scheme("SCT")
	all := sct.Dir()
	if !sort.StringsAreSorted(all) {
		t.Error("Dir() is not sorted")
	}
	if len(all) < 1000 {
		t.Errorf("SCT has %d keywords, expected thousands", len(all))
	}
}

func TestCollectionContains(t *testing.T) {
	right := New("24028007", "SCT", "Right")

	cid244, _ := CID(244)
	if !cid244.ContainsCode(right) {
		t.Error("CID244 does not hold (SCT, 24028007, Right)")
	}
	if !cid244.ContainsKeyword("Right") {
		t.Error("CID244 has no keyword Right")
	}
	if cid244.ContainsKeyword("Foo") {
		t.Error("CID244 has a keyword Foo")
	}

	// The meaning is not part of the comparison, so a code that says something
	// else about the same value is still in the collection.
	if !cid244.ContainsCode(New("24028007", "SCT", "something else")) {
		t.Error("ContainsCode compares meanings")
	}
	if cid244.ContainsCode(New("99999999", "SCT", "Right")) {
		t.Error("CID244 holds an unrelated value")
	}

	sct, _ := Scheme("SCT")
	if !sct.ContainsCode(right) || sct.ContainsCode(New("24028007", "DCM", "Right")) {
		t.Error("SCT contains the wrong codes")
	}
}

func TestCollectionConcepts(t *testing.T) {
	ucum, _ := Scheme("UCUM")
	concepts := ucum.Concepts()
	if got, want := concepts["Second"], New("s", "UCUM", "second"); got != want {
		t.Fatalf("UCUM Second = %v, want %v", got, want)
	}
	if len(concepts) != len(ucum.Dir()) {
		t.Fatalf("Concepts() has %d entries, Dir() %d", len(concepts), len(ucum.Dir()))
	}

	// The result is built once and handed out again, not rebuilt per call.
	if len(ucum.Concepts()) != len(concepts) {
		t.Fatal("Concepts() is not stable")
	}
}

// TestCollectionString compares the table with what pydicom prints for CID606,
// the smallest context with codes in it, character for character.
func TestCollectionString(t *testing.T) {
	cid, err := CID(606)
	if err != nil {
		t.Fatalf("CID(606): %v", err)
	}

	want := `CID606
Attribute             Code        Scheme   Meaning
---------             ----        ------   -------
Igloo                 127220      DCM      Igloo
None_                 260413007   SCT      None
RedTranslucentIgloo   127221      DCM      Red translucent igloo`

	if got := cid.String(); got != want {
		t.Errorf("CID606:\n%s\n\nwant:\n%s", got, want)
	}
}

// TestCollectionStringOfAScheme checks the narrower table a scheme prints: no
// scheme column, and the designator on the first line.
func TestCollectionStringOfAScheme(t *testing.T) {
	ucum, _ := Scheme("UCUM")
	lines := strings.Split(ucum.String(), "\n")
	if lines[0] != "Scheme: UCUM" {
		t.Fatalf("first line = %q", lines[0])
	}
	if !strings.HasPrefix(lines[1], "Attribute ") || !strings.Contains(lines[1], " Meaning") {
		t.Fatalf("header = %q", lines[1])
	}
	if !strings.Contains(ucum.String(), "\nArbitraryUnit ") {
		t.Fatal("the table has no ArbitraryUnit row")
	}
}

// TestCollectionStringOfAnEmptyCID covers the twelve contexts PS3.16 names but
// gives no codes. pydicom raises on those; printing the header is more useful
// than that.
func TestCollectionStringOfAnEmptyCID(t *testing.T) {
	cid, err := CID(101)
	if err != nil {
		t.Fatalf("CID(101): %v", err)
	}
	if len(cid.Dir()) != 0 {
		t.Fatalf("CID101 has %d keywords", len(cid.Dir()))
	}
	if got := cid.String(); got != "CID101\nAttribute   Code   Scheme   Meaning\n---------   ----   ------   -------" {
		t.Fatalf("CID101:\n%q", got)
	}
	if len(cid.Concepts()) != 0 {
		t.Fatal("CID101 has concepts")
	}
}

// TestEveryCollectionPrints walks the table once more, printing each
// collection. A row that is missing its code, or a width computed in bytes
// where pydicom counts characters, shows up as a table that does not have one
// line per keyword.
func TestEveryCollectionPrints(t *testing.T) {
	codes := Codes()

	for _, name := range codes.Collections() {
		collection, err := codes.Get(name)
		if err != nil {
			t.Fatalf("Get(%q): %v", name, err)
		}

		table := collection.String()
		lines := strings.Split(table, "\n")
		header := name
		if !collection.IsCID() {
			header = "Scheme: " + name
		}
		if lines[0] != header {
			t.Errorf("%s: first line = %q, want %q", name, lines[0], header)
		}

		rows := len(collection.Concepts())
		if len(lines) != rows+3 {
			t.Errorf("%s: table has %d lines for %d codes", name, len(lines), rows)
		}
		for _, line := range lines[3:] {
			if strings.HasSuffix(line, " ") || strings.Contains(line, "  \t") {
				t.Errorf("%s: row %q is not padded to a full column", name, line)
			}
		}
	}
}

func TestConcepts(t *testing.T) {
	codes := Codes()

	schemes := codes.Schemes()
	want := []string{"DCM", "SCT", "NCIt", "RXNORM", "RADLEX", "MSH", "UCUM", "LN",
		"IBSI", "UMLS", "PUBCHEM_CID", "FMA", "MDC", "BARI", "I10", "ITIS_TSN", "NEU", "NDC"}
	if strings.Join(schemes, ",") != strings.Join(want, ",") {
		t.Fatalf("Schemes() = %v, want %v", schemes, want)
	}

	cids := codes.CIDs()
	if len(cids) != 1355 {
		t.Fatalf("CIDs() has %d entries, want 1355", len(cids))
	}
	if cids[0] != "CID2" {
		t.Fatalf("first CID = %q", cids[0])
	}

	collections := codes.Collections()
	if len(collections) != len(schemes)+len(cids) {
		t.Fatalf("Collections() has %d entries, want %d", len(collections), len(schemes)+len(cids))
	}
	if collections[0] != "DCM" || collections[len(schemes)] != "CID2" {
		t.Fatalf("Collections() starts %v... then %q", collections[:2], collections[len(schemes)])
	}

	// Get hands back the collection Collections() named, not a new one, so the
	// work behind Concepts() is done once.
	first, err := codes.Get("SCT")
	if err != nil {
		t.Fatalf("Get(SCT): %v", err)
	}
	second, err := codes.Get("SCT")
	if err != nil {
		t.Fatalf("Get(SCT): %v", err)
	}
	if first != second {
		t.Fatal("Get(SCT) returned two different collections")
	}
	if got, err := codes.Get("cid2"); err != nil || got.Name() != "CID2" {
		t.Fatalf("Get(cid2) = %v, %v", got, err)
	}
	if _, err := codes.Get("sct"); !errors.Is(err, ErrNoSuchCollection) {
		t.Fatalf("Get(sct) error = %v", err)
	}
}

// TestParseCodes covers the record format on a table small enough to check by
// hand: the embedded file is the same parser, nine thousand times longer.
func TestParseCodes(t *testing.T) {
	source := strings.Join([]string{
		"# a comment",
		"",
		"S\tTEST\tSecondThing\tBAR\t2,3\tsecond thing",
		"S\tTEST\tFirstThing\tBAZ\t\tfirst thing",
		"S\tOTHER\tOnlyThing\tQUX\t\t only thing",
		"C\t2\tTwo",
		"C\t3\tThree",
		"I\t2\tTEST\tSecondThing",
		"I\t2\tTEST\tFirstThing",
		"I\t3\tOTHER\tOnlyThing",
		"M\tR-1\t99",
	}, "\n")

	tbl, err := parse(source)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	if strings.Join(tbl.schemes, ",") != "TEST,OTHER" {
		t.Fatalf("schemes = %v", tbl.schemes)
	}
	if got := len(tbl.concepts["TEST"]); got != 2 {
		t.Fatalf("TEST has %d keywords", got)
	}
	if got := tbl.concepts["TEST"]["SecondThing"][0]; got.code != (Code{"BAR", "TEST", "second thing", ""}) || len(got.cids) != 2 {
		t.Fatalf("SecondThing = %+v", got)
	}
	if got := tbl.concepts["OTHER"]["OnlyThing"][0]; got.code.Meaning != " only thing" || got.cids != nil {
		t.Fatalf("OnlyThing = %+v", got)
	}
	if len(tbl.cidNames) != 2 || tbl.cidNames[2] != "Two" || tbl.cidNames[3] != "Three" {
		t.Fatalf("CID names = %v", tbl.cidNames)
	}
	if strings.Join(tbl.members[2]["TEST"], ",") != "SecondThing,FirstThing" {
		t.Fatalf("CID2 members = %v", tbl.members)
	}
	if got := strings.Join(tbl.memberOrder[2], ","); got != "TEST" {
		t.Fatalf("CID2 draws on %q, want TEST", got)
	}
	if tbl.srtToSCT["R-1"] != "99" {
		t.Fatalf("SRT mapping = %v", tbl.srtToSCT)
	}
}

func TestParseCodesErrors(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{"empty", "", "no schemes"},
		{"no tab", "S scheme", "malformed record"},
		{"unknown kind", "X\ta\tb", "unknown record type"},
		{"short S record", "S\tTEST\tThird", "has 2 fields, want 5"},
		{"bad CID number", "S\tTEST\tThird\tVAL\tx\tmeaning", "bad CID number"},
		{"short C record", "C\t2", "has 1 fields, want 2"},
		{"non-numeric CID", "C\ttwo\tName", "record"},
		{"short I record", "I\t2\tTEST", "has 2 fields, want 3"},
		{"short M record", "M\tR-1", "has 1 fields, want 2"},
	}

	for _, test := range tests {
		_, err := parse(test.source)
		if err == nil || !strings.Contains(err.Error(), test.want) {
			t.Errorf("%s: error = %v, want it to contain %q", test.name, err, test.want)
		}
	}
}

// TestTableIsConsistent checks the embedded terminology against itself. The
// concept records and the CID records say the same thing from opposite ends --
// a code lists the CIDs it is in, a CID lists the keywords in it -- so a
// generator that drops or invents a link shows up here. The counts are the ones
// pydicom 3.1.0.dev0 ships; a new edition of PS3.16 changes them, and the test
// failing is the point at which the change should be looked at.
func TestTableIsConsistent(t *testing.T) {
	tbl := data()

	if got := len(tbl.schemes); got != 18 {
		t.Errorf("schemes = %d, want 18", got)
	}

	keywords, codes := 0, 0
	for scheme, byKeyword := range tbl.concepts {
		keywords += len(byKeyword)
		for keyword, entries := range byKeyword {
			codes += len(entries)
			for _, entry := range entries {
				if entry.code.SchemeDesignator != scheme {
					t.Fatalf("%s/%s holds a %s code", scheme, keyword, entry.code.SchemeDesignator)
				}
				for _, cid := range entry.cids {
					members := tbl.members[cid][scheme]
					found := false
					for _, member := range members {
						if member == keyword {
							found = true
							break
						}
					}
					if !found {
						t.Fatalf("%s/%s says it is in CID%d, which does not list it", scheme, keyword, cid)
					}
				}
			}
		}
	}
	if keywords != 18735 {
		t.Errorf("keywords = %d, want 18735", keywords)
	}
	if codes != 18735 {
		t.Errorf("codes = %d, want 18735", codes)
	}

	members := 0
	for cid, byScheme := range tbl.members {
		for scheme, cidKeywords := range byScheme {
			if _, ok := tbl.concepts[scheme]; !ok {
				t.Errorf("CID%d lists scheme %q, which has no concepts", cid, scheme)
			}
			for _, keyword := range cidKeywords {
				members++
				entries := tbl.concepts[scheme][keyword]
				if len(entries) == 0 {
					t.Fatalf("CID%d lists %s/%s, which is not in the table", cid, scheme, keyword)
				}
				listed := false
				for _, entry := range entries {
					for _, held := range entry.cids {
						if held == cid {
							listed = true
						}
					}
				}
				if !listed {
					t.Fatalf("CID%d lists %s/%s, which says it is in no such CID", cid, scheme, keyword)
				}
			}
		}
	}
	if members != 27033 {
		t.Errorf("CID memberships = %d, want 27033", members)
	}
	if got := len(tbl.cidNames); got != 1355 {
		t.Errorf("CIDs = %d, want 1355", got)
	}
	if got := len(tbl.cidOrder); got != 1355 {
		t.Errorf("CID order has %d entries, want 1355", got)
	}
	if got := len(tbl.srtToSCT); got != 7990 {
		t.Errorf("SRT/SCT equivalences = %d, want 7990", got)
	}
}

// TestEveryKeywordResolves walks the whole terminology: every keyword of every
// context has to come back as a code, and every keyword of every scheme too.
// pydicom would raise on the first one that does not; so does this.
func TestEveryKeywordResolves(t *testing.T) {
	codes := Codes()

	// The two keywords that reach two schemes, which pydicom raises on too.
	ambiguous := map[string]bool{
		"CID8134/ArcuateFasciculus":              true,
		"CID8134/SuperiorLongitudinalFasciculus": true,
	}

	resolved, refused := 0, 0
	for _, name := range codes.Collections() {
		collection, err := codes.Get(name)
		if err != nil {
			t.Fatalf("Get(%q): %v", name, err)
		}
		for _, keyword := range collection.Dir() {
			code, err := collection.Lookup(keyword)
			if err != nil {
				if !ambiguous[name+"/"+keyword] || !errors.Is(err, ErrAmbiguousCode) {
					t.Fatalf("%s.Lookup(%q): %v", name, keyword, err)
				}
				refused++
				continue
			}
			if ambiguous[name+"/"+keyword] {
				t.Fatalf("%s.Lookup(%q) resolved to %v, want an ambiguity", name, keyword, code)
			}
			// The value may be empty -- one LOINC entry in pydicom's data has
			// none, see TestOneCodeHasNoValue -- but a code without a scheme
			// or a meaning is a broken record.
			if code.SchemeDesignator == "" || code.Meaning == "" {
				t.Fatalf("%s/%s resolved to an incomplete code %v", name, keyword, code)
			}
			resolved++
		}
	}

	// 18735 keywords over the schemes and 27033 CID memberships, less the two
	// memberships that repeat a keyword within CID8134 (Dir is a set) and less
	// the two keywords that stay ambiguous.
	if want := 18735 + 27033 - 2 - 2; resolved != want {
		t.Errorf("resolved %d keywords, want %d", resolved, want)
	}
	if refused != 2 {
		t.Errorf("refused %d keywords as ambiguous, want 2", refused)
	}
}

// TestAmbiguousKeywordsInCID8134 covers the only two keywords in the
// terminology that reach two schemes at once, both in the white matter tract
// context. pydicom raises RuntimeError for them and names the schemes; the
// messages here are its messages, in its order.
func TestAmbiguousKeywordsInCID8134(t *testing.T) {
	cid, err := CID(8134)
	if err != nil {
		t.Fatalf("CID(8134): %v", err)
	}

	tests := []struct {
		keyword string
		want    string
	}{
		{"ArcuateFasciculus", `for keyword "ArcuateFasciculus" in CID8134: FMA, NEU all contain it`},
		{"SuperiorLongitudinalFasciculus", `for keyword "SuperiorLongitudinalFasciculus" in CID8134: NEU, SCT all contain it`},
	}

	for _, test := range tests {
		_, err := cid.Lookup(test.keyword)
		if !errors.Is(err, ErrAmbiguousCode) {
			t.Fatalf("Lookup(%q) error = %v", test.keyword, err)
		}
		if !strings.Contains(err.Error(), test.want) {
			t.Errorf("Lookup(%q) error = %q, want it to contain %q", test.keyword, err, test.want)
		}
		if !cid.ContainsKeyword(test.keyword) {
			t.Errorf("CID8134 does not list %q in Dir()", test.keyword)
		}
	}

	// Both are in Dir() -- the CID does list them -- but neither is in
	// Concepts(), because there is no single code to put there.
	if len(cid.Dir()) != len(cid.Concepts())+2 {
		t.Fatalf("Dir() has %d keywords, Concepts() %d", len(cid.Dir()), len(cid.Concepts()))
	}
	if strings.Contains(cid.String(), "\nArcuateFasciculus ") {
		t.Fatal("the table has an ArcuateFasciculus row, with no code in it")
	}
}

// TestOneCodeHasNoValue pins the one entry in pydicom's terminology whose code
// value is an empty string: the LOINC code for the main pulmonary artery
// velocity maximum is missing from the extraction pydicom ships. The port
// carries it as it is rather than inventing a value; if a later pydicom fills
// it in, this test says so.
func TestOneCodeHasNoValue(t *testing.T) {
	tbl := data()

	var empty []string
	for scheme, byKeyword := range tbl.concepts {
		for keyword, entries := range byKeyword {
			for _, entry := range entries {
				if entry.code.Value == "" {
					empty = append(empty, scheme+"/"+keyword)
				}
			}
		}
	}

	if len(empty) != 1 || empty[0] != "LN/MainPulmonaryArteryVmax" {
		t.Fatalf("codes with no value: %v", empty)
	}

	ln, err := Scheme("LN")
	if err != nil {
		t.Fatal(err)
	}
	code, err := ln.Lookup("MainPulmonaryArteryVmax")
	if err != nil {
		t.Fatal(err)
	}
	if code.Value != "" || code.Meaning != "Main pulmonary artery Vmax" {
		t.Fatalf("code = %v", code)
	}
}

// contains reports whether list holds want.
func contains(list []string, want string) bool {
	for _, item := range list {
		if item == want {
			return true
		}
	}

	return false
}

func BenchmarkLookup(b *testing.B) {
	sct, err := Scheme("SCT")
	if err != nil {
		b.Fatal(err)
	}
	for b.Loop() {
		if _, err := sct.Lookup("Lung"); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkCIDLookup(b *testing.B) {
	cid, err := CID(2)
	if err != nil {
		b.Fatal(err)
	}
	for b.Loop() {
		if _, err := cid.Lookup("Transverse"); err != nil {
			b.Fatal(err)
		}
	}
}

func ExampleCodes() {
	lung, err := Scheme("SCT")
	if err != nil {
		fmt.Println(err)
		return
	}
	code, err := lung.Lookup("Lung")
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(code)

	context, err := CID(2)
	if err != nil {
		fmt.Println(err)
		return
	}
	transverse, err := context.Lookup("Transverse")
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(transverse)

	// Output:
	// (SCT, "39607008", "Lung")
	// (SCT, "62824007", "Transverse")
}
