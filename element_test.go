package godicom

import (
	"strings"
	"testing"
)

func TestDataElementCreation(t *testing.T) {
	elem := NewDataElement(MustTag(0x00100010), VRPN, "Test^Name")
	if elem.Tag != MustTag(0x00100010) {
		t.Error("wrong tag")
	}
	if elem.VR != VRPN {
		t.Error("wrong VR")
	}
	if elem.Value != "Test^Name" {
		t.Error("wrong value")
	}
}

func TestDataElementVM(t *testing.T) {
	elem := NewDataElement(MustTag(0x00100010), VRPN, "Test")
	if elem.VM() != 1 {
		t.Errorf("VM = %d, want 1", elem.VM())
	}
	elem2 := NewDataElement(MustTag(0x00280010), VRUS, 512)
	if elem2.VM() != 1 {
		t.Errorf("VM = %d, want 1", elem2.VM())
	}
}

func TestDataElementEmpty(t *testing.T) {
	elem := NewDataElement(MustTag(0x00100010), VRPN, "")
	if !elem.IsEmpty() {
		t.Error("empty PN should be empty")
	}
	elem2 := NewDataElement(MustTag(0x00280010), VRUS, nil)
	if !elem2.IsEmpty() {
		t.Error("nil value should be empty")
	}
}

func TestDataElementName(t *testing.T) {
	elem := NewDataElement(MustTag(0x00100010), VRPN, "Test")
	if elem.Name() != "Patient's Name" {
		t.Errorf("Name = %q, want Patient's Name", elem.Name())
	}
}

func TestDataElementKeyword(t *testing.T) {
	elem := NewDataElement(MustTag(0x00100010), VRPN, "Test")
	if elem.Keyword() != "PatientName" {
		t.Errorf("Keyword = %q, want PatientName", elem.Keyword())
	}
}

func TestDataElementPrivate(t *testing.T) {
	elem := NewDataElement(MustTag(0x00090010), VRLO, "Private")
	if !elem.IsPrivate() {
		t.Error("private tag should be private")
	}
}

func TestDataElementReprValue(t *testing.T) {
	longString := strings.Repeat("a", 70)
	seq := NewSequence([]*Dataset{NewDataset(), NewDataset()})
	manyValues := []interface{}{}
	for i := range 17 {
		manyValues = append(manyValues, i)
	}

	tests := []struct {
		name     string
		element  *DataElement
		expected string
	}{
		{
			name:     "nil",
			element:  NewDataElement(MustTag(0x00100010), VRPN, nil),
			expected: "",
		},
		{
			name:     "uid",
			element:  NewDataElement(MustTag(0x00020010), VRUI, ExplicitVRLittleEndian),
			expected: "Explicit VR Little Endian",
		},
		{
			name:     "sequence",
			element:  NewDataElement(MustTag(0x300A00B0), VRSQ, seq),
			expected: "Sequence of 2 items",
		},
		{
			name:     "short bytes",
			element:  NewDataElement(MustTag(0x7FE00010), VROB, []byte{1, 2, 3}),
			expected: "[1 2 3]",
		},
		{
			name:     "long bytes",
			element:  NewDataElement(MustTag(0x7FE00010), VROB, make([]byte, 17)),
			expected: "Array of 17 bytes",
		},
		{
			name:     "multi value",
			element:  NewDataElement(MustTag(0x00100020), VRLO, NewMultiValue([]interface{}{"A", "B"})),
			expected: "[A B]",
		},
		{
			name:     "long multi value",
			element:  NewDataElement(MustTag(0x00100020), VRLO, NewMultiValue(manyValues)),
			expected: "Array of 17 elements",
		},
		{
			name:     "long string",
			element:  NewDataElement(MustTag(0x00100010), VRPN, longString),
			expected: strings.Repeat("a", 61) + "...",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.element.ReprValue(); got != tt.expected {
				t.Fatalf("ReprValue = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestDataElementString(t *testing.T) {
	elem := NewDataElement(MustTag(0x00100010), VRPN, "Test^Name")
	got := elem.String()
	if !strings.Contains(got, "(0010,0010)") {
		t.Fatalf("String = %q, want tag", got)
	}
	if !strings.Contains(got, "Patient's Name") {
		t.Fatalf("String = %q, want element name", got)
	}
	if !strings.Contains(got, "PN: Test^Name") {
		t.Fatalf("String = %q, want VR and value", got)
	}
}

func TestDataElementNameFallbacks(t *testing.T) {
	tests := []struct {
		name     string
		element  *DataElement
		expected string
	}{
		{
			name:     "private creator",
			element:  NewDataElement(MustTag(0x00090010), VRLO, "CREATOR"),
			expected: "Private Creator",
		},
		{
			name:     "private data",
			element:  NewDataElement(MustTag(0x00091001), VRLO, "VALUE"),
			expected: "Private tag data",
		},
		{
			name:     "group length",
			element:  NewDataElement(MustTag(0x00100000), VRUL, 0),
			expected: "Group Length",
		},
		{
			name:     "unknown public",
			element:  NewDataElement(MustTag(0x00100001), VRUN, nil),
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.element.Name(); got != tt.expected {
				t.Fatalf("Name = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestDataElementVMAdditionalTypes(t *testing.T) {
	seq := NewSequence([]*Dataset{NewDataset(), NewDataset()})
	tests := []struct {
		name     string
		element  *DataElement
		expected int
	}{
		{
			name:     "sq sequence",
			element:  NewDataElement(MustTag(0x300A00B0), VRSQ, seq),
			expected: 2,
		},
		{
			name:     "sq non sequence",
			element:  NewDataElement(MustTag(0x300A00B0), VRSQ, nil),
			expected: 0,
		},
		{
			name:     "multi value string",
			element:  NewDataElement(MustTag(0x00100020), VRLO, NewMultiValue([]string{"A", "B"})),
			expected: 2,
		},
		{
			name:     "multi value int",
			element:  NewDataElement(MustTag(0x00280010), VRUS, NewMultiValue([]int{1, 2, 3})),
			expected: 3,
		},
		{
			name:     "multi value float",
			element:  NewDataElement(MustTag(0x00186050), VRDS, NewMultiValue([]float64{1.1, 2.2})),
			expected: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.element.VM(); got != tt.expected {
				t.Fatalf("VM = %d, want %d", got, tt.expected)
			}
		})
	}
}

func TestDataElementEqual(t *testing.T) {
	a := NewDataElement(MustTag(0x00100010), VRPN, "Test^Name")
	b := NewDataElement(MustTag(0x00100010), VRPN, "Test^Name")
	if !a.Equal(b) {
		t.Fatal("Equal = false, want true")
	}

	differentTag := NewDataElement(MustTag(0x00100020), VRPN, "Test^Name")
	if a.Equal(differentTag) {
		t.Fatal("Equal with different tag = true, want false")
	}

	differentVR := NewDataElement(MustTag(0x00100010), VRLO, "Test^Name")
	if a.Equal(differentVR) {
		t.Fatal("Equal with different VR = true, want false")
	}

	differentValue := NewDataElement(MustTag(0x00100010), VRPN, "Other^Name")
	if a.Equal(differentValue) {
		t.Fatal("Equal with different value = true, want false")
	}
}
