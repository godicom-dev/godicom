package godicom

import (
	"strings"
	"testing"
)

// The cases here are ported from pydicom tests/test_dataelem.py. Subtest names
// follow the pydicom test function so a grep finds the origin of each claim.
// Python-protocol cases (equality with tuples, hash, pickling, __getitem__) have
// no Go counterpart and are deliberately absent.

func TestDataElementVMPerValueType(t *testing.T) {
	// pydicom: TestDataElement::test_VM_1, test_VM_2, test_vm_sequence
	tests := []struct {
		name string
		elem *Element
		want int
	}{
		{"test_VM_1_single_string", NewElement(MustTag(0x00100010), VRPN, "Test"), 1},
		{"test_VM_1_empty_string", NewElement(MustTag(0x00100010), VRPN, ""), 0},
		{"test_VM_1_nil", NewElement(MustTag(0x00100010), VRPN, nil), 0},
		{"test_VM_1_us", NewElement(MustTag(0x00280010), VRUS, 512), 1},
		{"test_VM_2_multivalue_string", NewElement(MustTag(0x00100020), VRLO, NewMultiValue([]string{"A", "B"})), 2},
		{"test_VM_2_multivalue_int", NewElement(MustTag(0x00280010), VRUS, NewMultiValue([]int{1, 2, 3})), 3},
		{"test_VM_2_empty_multivalue", NewElement(MustTag(0x00100020), VRLO, NewMultiValue([]string{})), 0},
		{"test_VM_2_empty_bytes", NewElement(MustTag(0x7FE00010), VROB, []byte{}), 0},
		{"test_VM_2_bytes", NewElement(MustTag(0x7FE00010), VROB, []byte{1, 2}), 1},
		{"test_VM_1_zero_person_name", NewElement(MustTag(0x00100010), VRPN, PersonName{}), 0},
		{"test_VM_1_person_name", NewElement(MustTag(0x00100010), VRPN, ParsePersonName("A^B")), 1},
		{"test_VM_2_multivalue_person_name", NewElement(MustTag(0x00100010), VRPN, NewMultiValue([]PersonName{{Alphabetic: "A"}, {Alphabetic: "B"}})), 2},
		{"test_VM_2_multivalue_tag", NewElement(MustTag(0x00081115), VRAT, NewMultiValue([]Tag{MustTag(0x00100010), MustTag(0x00100020)})), 2},
		{"test_vm_sequence_without_value", NewElement(MustTag(0x0040A730), VRSQ, nil), 0},
		{"test_vm_sequence_non_sequence_value", NewElement(MustTag(0x0040A730), VRSQ, "not a sequence"), 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.elem.VM(); got != tt.want {
				t.Fatalf("VM() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestDataElementVMSingleItemSequence(t *testing.T) {
	// pydicom: TestDataElement::test_vm_sequence
	seq := NewSequence([]*Dataset{NewDataset()})
	elem := NewElement(MustTag(0x0040A730), VRSQ, seq)
	if got := elem.VM(); got != 1 {
		t.Fatalf("VM() = %d, want 1", got)
	}
}

func TestDataElementIsEmpty(t *testing.T) {
	// pydicom: TestDataElement::test_empty_text_values, test_empty_binary_values
	tests := []struct {
		name string
		elem *Element
		want bool
	}{
		{"test_empty_text_values_empty_string", NewElement(MustTag(0x00100010), VRPN, ""), true},
		{"test_empty_text_values_nil", NewElement(MustTag(0x00100010), VRPN, nil), true},
		{"test_empty_binary_values_empty_slice", NewElement(MustTag(0x7FE00010), VROB, []byte{}), true},
		{"test_empty_binary_values_nonempty_slice", NewElement(MustTag(0x7FE00010), VROB, []byte{0}), false},
		{"test_is_empty_sequence_empty", NewElement(MustTag(0x0040A730), VRSQ, NewSequence(nil)), true},
		{"test_is_empty_sequence_one_item", NewElement(MustTag(0x0040A730), VRSQ, NewSequence([]*Dataset{NewDataset()})), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.elem.IsEmpty(); got != tt.want {
				t.Fatalf("IsEmpty() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDataElementPrivateTags(t *testing.T) {
	// pydicom: TestDataElement::test_is_private, test_private_tag_in_repeater_range
	tests := []struct {
		name string
		tag  Tag
		want bool
	}{
		{"test_is_private_standard", MustTag(0x00100010), false},
		{"test_is_private_odd_group", MustTag(0x00090010), true},
		{"test_is_private_creator_range", MustTag(0x00090010), true},
		{"test_private_tag_in_repeater_range_even_group", MustTag(0x00281010), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			elem := NewElement(tt.tag, VRLO, "x")
			if got := elem.IsPrivate(); got != tt.want {
				t.Fatalf("IsPrivate() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDataElementNameAndKeyword(t *testing.T) {
	// pydicom: TestDataElement::test_name, test_keyword, test_name_group_length,
	// test_name_unknown, test_name_unknown_private, test_retired
	tests := []struct {
		name        string
		tag         Tag
		wantName    string
		wantKeyword string
	}{
		{"test_name_patient_name", MustTag(0x00100010), "Patient's Name", "PatientName"},
		{"test_name_group_length", MustTag(0x00100000), "Group Length", ""},
		{"test_name_unknown", MustTag(0x00100001), "", ""},
		{"test_name_unknown_private", MustTag(0x00091001), "Private tag data", ""},
		{"test_name_private_creator", MustTag(0x00090010), "Private Creator", ""},
		{"test_keyword_pixel_data", MustTag(0x7FE00010), "Pixel Data", "PixelData"},
		{"test_keyword_sop_class_uid", MustTag(0x00080016), "SOP Class UID", "SOPClassUID"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			elem := NewElement(tt.tag, VRLO, "x")
			if got := elem.Name(); got != tt.wantName {
				t.Fatalf("Name() = %q, want %q", got, tt.wantName)
			}
			if got := elem.Keyword(); got != tt.wantKeyword {
				t.Fatalf("Keyword() = %q, want %q", got, tt.wantKeyword)
			}
		})
	}
}

func TestDataElementReprValueTypes(t *testing.T) {
	// pydicom: TestDataElement::test_repval_large_elem, test_repval_large_vm,
	// test_repval_strange_type, test_UID, test_repr_seq
	long := strings.Repeat("a", 70)
	tests := []struct {
		name string
		elem *Element
		want string
	}{
		{"test_UID_typed_value", NewElement(MustTag(0x00020010), VRUI, ExplicitVRLittleEndian), "Explicit VR Little Endian"},
		{"test_UID_string_value", NewElement(MustTag(0x00020010), VRUI, "1.2.840.10008.1.2.1"), "Explicit VR Little Endian"},
		{"test_repval_seq", NewElement(MustTag(0x300A00B0), VRSQ, NewSequence([]*Dataset{NewDataset()})), "Sequence of 1 items"},
		{"test_repval_large_elem_bytes", NewElement(MustTag(0x7FE00010), VROB, make([]byte, 17)), "Array of 17 bytes"},
		{"test_repval_strange_type_small", NewElement(MustTag(0x00280010), VRUS, 42), "42"},
		{"test_repval_large_vm", NewElement(MustTag(0x00100020), VRLO, NewMultiValue(make17())), "Array of 17 elements"},
		{"test_repval_long_string", NewElement(MustTag(0x00100010), VRPN, long), strings.Repeat("a", 61) + "..."},
		{"test_repval_person_name", NewElement(MustTag(0x00100010), VRPN, ParsePersonName("Doe^John")), "Doe^John"},
		{"test_repval_nil", NewElement(MustTag(0x00100010), VRPN, nil), ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.elem.ReprValue(); got != tt.want {
				t.Fatalf("ReprValue() = %q, want %q", got, tt.want)
			}
		})
	}
}

func make17() []interface{} {
	out := make([]interface{}, 17)
	for i := range out {
		out[i] = i
	}
	return out
}

func TestDataElementEquality(t *testing.T) {
	// pydicom: TestDataElement::test_equality_standard_element,
	// test_equality_private_element, test_equality_sequence_element,
	// test_equality_not_rlement, test_inequality_standard, test_inequality_sequence
	privateA := NewElement(MustTag(0x00091001), VRLO, "ACME 1.0")
	privateB := NewElement(MustTag(0x00091001), VRLO, "ACME 1.0")
	seqA := NewElement(MustTag(0x300A00B0), VRSQ, NewSequence([]*Dataset{datasetWithTag(0x00100010, "A")}))
	seqB := NewElement(MustTag(0x300A00B0), VRSQ, NewSequence([]*Dataset{datasetWithTag(0x00100010, "A")}))
	seqC := NewElement(MustTag(0x300A00B0), VRSQ, NewSequence([]*Dataset{datasetWithTag(0x00100010, "B")}))

	tests := []struct {
		name string
		a, b *Element
		want bool
	}{
		{"test_equality_standard_element", NewElement(MustTag(0x00100010), VRPN, "Doe^John"), NewElement(MustTag(0x00100010), VRPN, "Doe^John"), true},
		{"test_inequality_standard", NewElement(MustTag(0x00100010), VRPN, "Doe^John"), NewElement(MustTag(0x00100010), VRPN, "Doe^Jane"), false},
		{"test_equality_private_element", privateA, privateB, true},
		{"test_equality_sequence_element", seqA, seqB, true},
		{"test_inequality_sequence", seqA, seqC, false},
		{"test_equality_not_rlement_tag", NewElement(MustTag(0x00100010), VRPN, "x"), NewElement(MustTag(0x00100020), VRPN, "x"), false},
		{"test_equality_not_rlement_vr", NewElement(MustTag(0x00100010), VRPN, "x"), NewElement(MustTag(0x00100010), VRLO, "x"), false},
		{"test_equality_bytes", NewElement(MustTag(0x7FE00010), VROB, []byte{1, 2}), NewElement(MustTag(0x7FE00010), VROB, []byte{1, 2}), true},
		{"test_inequality_bytes", NewElement(MustTag(0x7FE00010), VROB, []byte{1, 2}), NewElement(MustTag(0x7FE00010), VROB, []byte{1, 3}), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.a.Equal(tt.b); got != tt.want {
				t.Fatalf("Equal() = %v, want %v", got, tt.want)
			}
			if got := tt.b.Equal(tt.a); got != tt.want {
				t.Fatalf("Equal() reversed = %v, want %v", got, tt.want)
			}
		})
	}
}

func datasetWithTag(t Tag, value string) *Dataset {
	ds := NewDataset()
	ds.SetString(t, value)
	return ds
}

func TestElementRawFlag(t *testing.T) {
	// Godicom-specific: a decoded value with no original bytes is not raw, and a
	// read element that kept its bytes is.
	elem := NewElement(MustTag(0x00100010), VRPN, "Doe^John")
	if elem.IsRaw() {
		t.Fatal("IsRaw() = true for a decoded element, want false")
	}

	read := NewElement(MustTag(0x00100010), VRPN, "Doe^John")
	read.RawValue = []byte("Doe^John")
	if !read.IsRaw() {
		t.Fatal("IsRaw() = false for an element holding RawValue, want true")
	}
}

func TestElementStringIncludesTagNameVRAndValue(t *testing.T) {
	elem := NewElement(MustTag(0x00100010), VRPN, "Doe^John")
	got := elem.String()
	for _, want := range []string{"(0010,0010)", "Patient's Name", "PN:", "Doe^John"} {
		if !strings.Contains(got, want) {
			t.Fatalf("String() = %q, missing %q", got, want)
		}
	}
}

func TestRawDataElementHasNoDecodedValue(t *testing.T) {
	// Mirrors the contract ReadOptions.OnElement hands a hook: Tag, VR, Length and
	// ValueTell are set, Value is nil because the point is not having read it.
	raw := RawDataElement{Tag: MustTag(0x7FE00010), VR: VROB, Length: 4, ValueTell: 132}
	if raw.Tag != MustTag(0x7FE00010) || raw.VR != VROB || raw.Length != 4 || raw.ValueTell != 132 {
		t.Fatalf("RawDataElement fields not preserved: %+v", raw)
	}
	if raw.Value != nil {
		t.Fatal("Value should stay nil on a raw element")
	}
}
