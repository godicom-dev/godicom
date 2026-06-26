package godicom

import (
	"testing"
)

func TestVRClassification(t *testing.T) {
	if !IsStringVR(VRAE) {
		t.Error("AE should be string VR")
	}
	if !IsBinaryVR(VROB) {
		t.Error("OB should be binary VR")
	}
	if !IsIntVR(VRUS) {
		t.Error("US should be int VR")
	}
	if !IsFloatVR(VRFD) {
		t.Error("FD should be float VR")
	}
	if !ExplicitVRLength16[VRUL] {
		t.Error("UL should have 16-bit explicit VR length")
	}
	if !ExplicitVRLength32[VROB] {
		t.Error("OB should have 32-bit explicit VR length")
	}
}

func TestStandardVRs(t *testing.T) {
	if !StandardVRs[VRAE] {
		t.Error("AE should be standard VR")
	}
	if StandardVRs["XX"] {
		t.Error("XX should not be standard VR")
	}
}

func TestBytesVR(t *testing.T) {
	byteVRs := []VR{VROB, VROD, VROF, VROL, VROW, VROV, VRUN}
	for _, vr := range byteVRs {
		if !BytesVR[vr] {
			t.Errorf("%s should be in BytesVR", vr)
		}
	}
}

func TestStrVR(t *testing.T) {
	if !StrVR[VRLO] {
		t.Error("LO should be string VR")
	}
	if !StrVR[VRPN] {
		t.Error("PN should be string VR")
	}
}
