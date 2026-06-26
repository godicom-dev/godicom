package godicom

// VR represents a DICOM Value Representation.
type VR string

const (
	VRAE VR = "AE"
	VRAS VR = "AS"
	VRAT VR = "AT"
	VRCS VR = "CS"
	VRDA VR = "DA"
	VRDS VR = "DS"
	VRDT VR = "DT"
	VRFD VR = "FD"
	VRFL VR = "FL"
	VRIS VR = "IS"
	VRLO VR = "LO"
	VRLT VR = "LT"
	VROB VR = "OB"
	VROD VR = "OD"
	VROF VR = "OF"
	VROL VR = "OL"
	VROW VR = "OW"
	VROV VR = "OV"
	VRPN VR = "PN"
	VRSH VR = "SH"
	VRSL VR = "SL"
	VRSQ VR = "SQ"
	VRSS VR = "SS"
	VRST VR = "ST"
	VRSV VR = "SV"
	VRTM VR = "TM"
	VRUC VR = "UC"
	VRUI VR = "UI"
	VRUL VR = "UL"
	VRUN VR = "UN"
	VRUR VR = "UR"
	VRUS VR = "US"
	VRUT VR = "UT"
	VRUV VR = "UV"
)

// StandardVRs contains all standard VRs.
var StandardVRs = map[VR]bool{
	VRAE: true, VRAS: true, VRAT: true, VRCS: true,
	VRDA: true, VRDS: true, VRDT: true, VRFD: true,
	VRFL: true, VRIS: true, VRLO: true, VRLT: true,
	VROB: true, VROD: true, VROF: true, VROL: true,
	VROW: true, VROV: true, VRPN: true, VRSH: true,
	VRSL: true, VRSQ: true, VRSS: true, VRST: true,
	VRSV: true, VRTM: true, VRUC: true, VRUI: true,
	VRUL: true, VRUN: true, VRUR: true, VRUS: true,
	VRUT: true, VRUV: true,
}

// VR classification sets
var (
	BytesVR          = map[VR]bool{VROB: true, VROD: true, VROF: true, VROL: true, VROV: true, VROW: true, VRUN: true}
	FloatVR          = map[VR]bool{VRDS: true, VRFD: true, VRFL: true}
	IntVR            = map[VR]bool{VRAT: true, VRIS: true, VRSL: true, VRSS: true, VRSV: true, VRUL: true, VRUS: true, VRUV: true}
	ListVR           = map[VR]bool{VRSQ: true}
	DefaultCharsetVR = map[VR]bool{VRAE: true, VRAS: true, VRCS: true, VRDA: true, VRDS: true, VRDT: true, VRIS: true, VRTM: true, VRUI: true, VRUR: true}
	CustomCharsetVR  = map[VR]bool{VRLO: true, VRLT: true, VRPN: true, VRSH: true, VRST: true, VRUC: true, VRUT: true}
	StrVR            = func() map[VR]bool {
		m := make(map[VR]bool)
		for k, v := range DefaultCharsetVR {
			m[k] = v
		}
		for k, v := range CustomCharsetVR {
			m[k] = v
		}
		return m
	}()
	AllowBackslash = func() map[VR]bool {
		m := make(map[VR]bool)
		for k, v := range BytesVR {
			m[k] = v
		}
		m[VRLT] = true
		m[VRST] = true
		m[VRUT] = true
		return m
	}()
)

// VRs that use 2-byte length fields for Explicit VR (Table 7.1-2, Part 5)
var ExplicitVRLength16 = map[VR]bool{
	VRAE: true, VRAS: true, VRAT: true, VRCS: true,
	VRDA: true, VRDS: true, VRDT: true, VRFL: true,
	VRFD: true, VRIS: true, VRLO: true, VRLT: true,
	VRPN: true, VRSH: true, VRSL: true, VRSS: true,
	VRST: true, VRTM: true, VRUI: true, VRUL: true,
	VRUS: true,
}

// VRs that use 4-byte length fields for Explicit VR
var ExplicitVRLength32 = func() map[VR]bool {
	m := make(map[VR]bool)
	for vr := range StandardVRs {
		if !ExplicitVRLength16[vr] {
			m[vr] = true
		}
	}
	return m
}()

// BufferableVRs are VRs that support buffered values.
var BufferableVRs = func() map[VR]bool {
	m := make(map[VR]bool)
	for vr := range BytesVR {
		if vr != VRUN {
			m[vr] = true
		}
	}
	return m
}()

// IsBinaryVR returns true if the VR is a binary (bytes-like) VR.
func IsBinaryVR(vr VR) bool { return BytesVR[vr] }

// IsStringVR returns true if the VR is a string-like VR.
func IsStringVR(vr VR) bool { return StrVR[vr] }

// IsIntVR returns true if the VR is an integer VR.
func IsIntVR(vr VR) bool { return IntVR[vr] }

// IsFloatVR returns true if the VR is a floating-point VR.
func IsFloatVR(vr VR) bool { return FloatVR[vr] }
