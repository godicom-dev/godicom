package dicomjson

import "github.com/godicom-dev/godicom"

func isBinaryVR(vr godicom.VR) bool {
	return godicom.BytesVR[vr] || vr == godicom.VRUN
}

func isIntegerVR(vr godicom.VR) bool {
	return godicom.IntVR[vr] || vr == godicom.VRUS || vr == godicom.VRSS || vr == godicom.VRUL || vr == godicom.VRSL
}

func isFloatVR(vr godicom.VR) bool {
	return godicom.FloatVR[vr] || vr == godicom.VRDS
}

func emptyJSONValue(vr godicom.VR) interface{} {
	if vr == godicom.VRSQ {
		return godicom.NewSequence(nil)
	}
	if vr == godicom.VRPN {
		return godicom.PersonName{}
	}
	if godicom.IsStringVR(vr) && vr != godicom.VRDS && vr != godicom.VRIS {
		return ""
	}
	return nil
}
