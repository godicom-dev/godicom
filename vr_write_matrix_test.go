package godicom

import (
	"path/filepath"
	"testing"
)

// vrSampleValues gives each standard VR a value of the Go type the writer
// documents for it, so the matrix below exercises the encoder rather than the
// conversion layer. pydicom covers the same ground one case per VR in
// TestWriteDataElement; covering them from one table makes the shape of the
// table itself the assertion -- a VR that gains a case above is a VR that
// writes.
var vrSampleValues = map[VR]any{
	VRAE: "AETITLE",
	VRAS: "032Y",
	VRAT: MustTag(0x00100010),
	VRCS: "TEST",
	VRDA: "20260101",
	VRDS: "1.5",
	VRDT: "20260101120000",
	VRFD: float64(1.5),
	VRFL: float32(1.5),
	VRIS: "42",
	VRLO: "TEXT",
	VRLT: "LONG TEXT",
	VROB: []byte{0x01, 0x02},
	VROD: []byte{0, 0, 0, 0, 0, 0, 0, 0},
	VROF: []byte{0, 0, 0, 0, 0, 0, 0, 0},
	VROL: []byte{0x01, 0, 0, 0},
	VROW: []byte{0x01, 0x00},
	VROV: []byte{0, 0, 0, 0, 0, 0, 0, 0},
	VRPN: "Doe^John",
	VRSH: "SHORT",
	VRSL: int32(-2),
	VRSQ: NewSequence(nil),
	VRSS: int16(-1),
	VRST: "SHORT TEXT",
	VRSV: int64(-1),
	VRTM: "120000",
	VRUC: "UNLIMITED",
	VRUI: "1.2.840.10008.1.1",
	VRUL: uint32(7),
	VRUN: []byte{0xDE, 0xAD},
	VRUR: "http://example.com",
	VRUS: uint16(3),
	VRUT: "UNLIMITED TEXT",
	VRUV: uint64(9),
}

// TestEveryStandardVRWritesAndReadsBack writes one element of every VR that the
// standard defines and reads it back from the file. The tag is private, so the
// dictionary cannot re-type the value on the way in and hide a writer that
// dropped the VR it was given.
func TestEveryStandardVRWritesAndReadsBack(t *testing.T) {
	for vr := range StandardVRs {
		value, ok := vrSampleValues[vr]
		if !ok {
			t.Errorf("no sample value for %s: add one to vrSampleValues", vr)
			continue
		}
		t.Run(string(vr), func(t *testing.T) {
			tag := MustTag(0x00710010)
			ds := NewDataset()
			ds.Set(NewDataElement(tag, vr, value))

			fd := samplePart10With(t, ds, ExplicitVRLittleEndian)
			path := filepath.Join(t.TempDir(), "vr.dcm")
			if err := fd.SaveAs(path, nil); err != nil {
				t.Fatalf("SaveAs() error = %v", err)
			}
			back, err := ReadFile(path, nil)
			if err != nil {
				t.Fatalf("ReadFile() error = %v", err)
			}
			elem, ok := back.Get(tag)
			if !ok {
				t.Fatalf("%s element is gone after the round trip", vr)
			}
			if elem.VR != vr {
				t.Errorf("VR = %s after the round trip, want %s", elem.VR, vr)
			}
		})
	}
}
