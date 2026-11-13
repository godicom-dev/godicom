package godicom

import (
	"fmt"
	"iter"
	"strings"

	"github.com/godicom-dev/godicom/tag"
	"github.com/godicom-dev/godicom/uid"
)

// This file builds the directory records a DICOMDIR is made of. It is the
// first half of the File-set writer: pydicom's fileset.py splits the same work
// between generate_filename(), is_conformant_file_id(), the _define_*
// recorders and FileSet._recordify(), and the split is kept here so the
// behaviour can be compared piece by piece.

// fileIDAlphanumeric holds the characters a File ID component may be built
// from. PS3.10 Section 8.5 allows [0-9][A-Z] and '_'; pydicom drops J from the
// alphanumeric set, and so does this list.
const fileIDAlphanumeric = "0123456789ABCDEFGHIKLMNOPQRSTUVWXYZ"

// fileIDNumeric is the numeric-only subset of fileIDAlphanumeric.
const fileIDNumeric = "0123456789"

// recordTypePrefixes maps a Directory Record Type to the two characters that
// start a generated File ID component for it.
var recordTypePrefixes = map[string]string{
	"PATIENT":          "PT",
	"STUDY":            "ST",
	"SERIES":           "SE",
	"IMAGE":            "IM",
	"RT DOSE":          "RD",
	"RT STRUCTURE SET": "RS",
	"RT PLAN":          "RP",
	"RT TREAT RECORD":  "RX",
	"PRESENTATION":     "PR",
	"WAVEFORM":         "WV",
	"SR DOCUMENT":      "SR",
	"KEY OBJECT DOC":   "KY",
	"SPECTROSCOPY":     "SP",
	"RAW DATA":         "RW",
	"REGISTRATION":     "RG",
	"FIDUCIAL":         "FD",
	"HANGING PROTOCOL": "HG",
	"ENCAP DOC":        "ED",
	"VALUE MAP":        "VM",
	"STEREOMETRIC":     "SX",
	"PALETTE":          "PA",
	"IMPLANT":          "IP",
	"IMPLANT ASSY":     "IA",
	"IMPLANT GROUP":    "IG",
	"PLAN":             "PL",
	"MEASUREMENT":      "MX",
	"SURFACE":          "SF",
	"SURFACE SCAN":     "SS",
	"TRACT":            "TR",
	"ASSESSMENT":       "AS",
	"RADIOTHERAPY":     "RT",
	"PRIVATE":          "P",
}

// GenerateFilename yields File IDs of eight characters each, increasing by one
// per item. A File ID is one component of the path to an instance inside a
// File-set, so it has to stay within the eight characters and the character
// set PS3.10 Section 8.5 allows.
//
// prefix is prepended to every ID and must be shorter than eight characters.
// start is the index of the first ID: with a start of 10 the first ID is
// "00000010". A negative start is treated as 0. When alphanumeric is false the
// IDs use [0-9] only, which tops out at 100 million IDs for an empty prefix;
// when true they use [0-9][A-I,K-Z], and J is skipped because pydicom skips it.
//
// The error is returned up front rather than on the first item, so a caller
// that ranges over the sequence never has to check mid-loop. pydicom raises
// from the generator on the first next() instead.
func GenerateFilename(prefix string, start int, alphanumeric bool) (iter.Seq[string], error) {
	if len(prefix) > 7 {
		return nil, fmt.Errorf("godicom: the prefix must be less than 8 characters long")
	}
	chars := fileIDNumeric
	if alphanumeric {
		chars = fileIDAlphanumeric
	}
	length := 8 - len(prefix)

	// 35^8 is 2.25e12, which does not fit an int on a 32-bit platform, so the
	// limit and the index are int64 regardless of GOARCH.
	base := int64(len(chars))
	limit := int64(1)
	for i := 0; i < length; i++ {
		limit *= base
	}

	idx := int64(start)
	if idx < 0 {
		idx = 0
	}

	return func(yield func(string) bool) {
		var suffix []byte
		for ; idx < limit; idx++ {
			n := idx
			suffix = suffix[:0]
			for n > 0 {
				suffix = append(suffix, chars[n%base])
				n /= base
			}
			// The digits come out least significant first.
			for i, j := 0, len(suffix)-1; i < j; i, j = i+1, j-1 {
				suffix[i], suffix[j] = suffix[j], suffix[i]
			}
			var sb strings.Builder
			sb.Grow(8)
			sb.WriteString(prefix)
			for i := len(suffix); i < length; i++ {
				sb.WriteByte('0')
			}
			sb.Write(suffix)
			if !yield(sb.String()) {
				return
			}
		}
	}, nil
}

// IsConformantFileID reports whether parts, taken as the components of a path
// relative to the File-set root, is a conformant File ID as PS3.10 Sections
// 8.2 and 8.5 define one: at most eight components, at most eight characters
// in a component, and only the ASCII characters [0-9][A-Z] and '_'.
//
// No components at all is conformant, matching pydicom, which applies the
// pattern to the empty string.
func IsConformantFileID(parts []string) bool {
	if len(parts) > 8 {
		return false
	}
	for _, part := range parts {
		if len(part) > 8 {
			return false
		}
		for i := 0; i < len(part); i++ {
			c := part[i]
			// A byte above 0x7F fails here, which is what the ASCII check in
			// pydicom's is_conformant_file_id() is for.
			if ('0' <= c && c <= '9') || ('A' <= c && c <= 'Z') || c == '_' {
				continue
			}
			return false
		}
	}
	return true
}

// singleLevelRecordTypeKeywords maps the SOP Class of a single-level instance
// to its Directory Record Type. The instance is filed directly under the root,
// with no PATIENT/STUDY/SERIES records above it.
var singleLevelRecordTypeKeywords = map[string]string{
	"HangingProtocolStorage":         "HANGING PROTOCOL",
	"ColorPaletteStorage":            "PALETTE",
	"GenericImplantTemplateStorage":  "IMPLANT",
	"ImplantAssemblyTemplateStorage": "IMPLANT ASSY",
	"ImplantTemplateGroupStorage":    "IMPLANT GROUP",
}

// fourLevelRecordTypeKeywords maps the SOP Class of an instance filed under
// PATIENT/STUDY/SERIES to its footer record type.
var fourLevelRecordTypeKeywords = map[string]string{
	"RTDoseStorage":                                           "RT DOSE",
	"RTStructureSetStorage":                                   "RT STRUCTURE SET",
	"RTBeamsTreatmentRecordStorage":                           "RT TREAT RECORD",
	"RTBrachyTreatmentRecordStorage":                          "RT TREAT RECORD",
	"RTTreatmentSummaryRecordStorage":                         "RT TREAT RECORD",
	"RTIonBeamsTreatmentRecordStorage":                        "RT TREAT RECORD",
	"GrayscaleSoftcopyPresentationStateStorage":               "PRESENTATION",
	"ColorSoftcopyPresentationStateStorage":                   "PRESENTATION",
	"PseudoColorSoftcopyPresentationStateStorage":             "PRESENTATION",
	"BlendingSoftcopyPresentationStateStorage":                "PRESENTATION",
	"XAXRFGrayscaleSoftcopyPresentationStateStorage":          "PRESENTATION",
	"BasicStructuredDisplayStorage":                           "PRESENTATION",
	"BasicVoiceAudioWaveformStorage":                          "WAVEFORM",
	"TwelveLeadECGWaveformStorage":                            "WAVEFORM",
	"GeneralECGWaveformStorage":                               "WAVEFORM",
	"AmbulatoryECGWaveformStorage":                            "WAVEFORM",
	"HemodynamicWaveformStorage":                              "WAVEFORM",
	"CardiacElectrophysiologyWaveformStorage":                 "WAVEFORM",
	"ArterialPulseWaveformStorage":                            "WAVEFORM",
	"RespiratoryWaveformStorage":                              "WAVEFORM",
	"GeneralAudioWaveformStorage":                             "WAVEFORM",
	"RoutineScalpElectroencephalogramWaveformStorage":         "WAVEFORM",
	"ElectromyogramWaveformStorage":                           "WAVEFORM",
	"ElectrooculogramWaveformStorage":                         "WAVEFORM",
	"SleepElectroencephalogramWaveformStorage":                "WAVEFORM",
	"MultichannelRespiratoryWaveformStorage":                  "WAVEFORM",
	"BodyPositionWaveformStorage":                             "WAVEFORM",
	"BasicTextSRStorage":                                      "SR DOCUMENT",
	"EnhancedSRStorage":                                       "SR DOCUMENT",
	"ComprehensiveSRStorage":                                  "SR DOCUMENT",
	"MammographyCADSRStorage":                                 "SR DOCUMENT",
	"ChestCADSRStorage":                                       "SR DOCUMENT",
	"ProcedureLogStorage":                                     "SR DOCUMENT",
	"XRayRadiationDoseSRStorage":                              "SR DOCUMENT",
	"SpectaclePrescriptionReportStorage":                      "SR DOCUMENT",
	"ColonCADSRStorage":                                       "SR DOCUMENT",
	"MacularGridThicknessAndVolumeReportStorage":              "SR DOCUMENT",
	"ImplantationPlanSRStorage":                               "SR DOCUMENT",
	"Comprehensive3DSRStorage":                                "SR DOCUMENT",
	"RadiopharmaceuticalRadiationDoseSRStorage":               "SR DOCUMENT",
	"ExtensibleSRStorage":                                     "SR DOCUMENT",
	"AcquisitionContextSRStorage":                             "SR DOCUMENT",
	"SimplifiedAdultEchoSRStorage":                            "SR DOCUMENT",
	"PatientRadiationDoseSRStorage":                           "SR DOCUMENT",
	"PlannedImagingAgentAdministrationSRStorage":              "SR DOCUMENT",
	"PerformedImagingAgentAdministrationSRStorage":            "SR DOCUMENT",
	"KeyObjectSelectionDocumentStorage":                       "KEY OBJECT DOC",
	"MRSpectroscopyStorage":                                   "SPECTROSCOPY",
	"RawDataStorage":                                          "RAW DATA",
	"SpatialRegistrationStorage":                              "REGISTRATION",
	"DeformableSpatialRegistrationStorage":                    "REGISTRATION",
	"SpatialFiducialsStorage":                                 "FIDUCIAL",
	"RealWorldValueMappingStorage":                            "VALUE MAP",
	"StereometricRelationshipStorage":                         "STEREOMETRIC",
	"LensometryMeasurementsStorage":                           "MEASUREMENT",
	"AutorefractionMeasurementsStorage":                       "MEASUREMENT",
	"KeratometryMeasurementsStorage":                          "MEASUREMENT",
	"SubjectiveRefractionMeasurementsStorage":                 "MEASUREMENT",
	"VisualAcuityMeasurementsStorage":                         "MEASUREMENT",
	"OphthalmicAxialMeasurementsStorage":                      "MEASUREMENT",
	"OphthalmicVisualFieldStaticPerimetryMeasurementsStorage": "MEASUREMENT",
	"SurfaceSegmentationStorage":                              "SURFACE",
	"SurfaceScanMeshStorage":                                  "SURFACE SCAN",
	"SurfaceScanPointCloudStorage":                            "SURFACE SCAN",
	"TractographyResultsStorage":                              "TRACT",
	"ContentAssessmentResultsStorage":                         "ASSESSMENT",
}

// singleLevelRecordTypes and fourLevelRecordTypes are the keyword tables
// resolved against the UID dictionary. pydicom keys these tables by UID
// directly; keeping the readable keyword in the source and resolving it here
// means a mistyped keyword fails loudly instead of quietly dropping a record
// type.
var (
	singleLevelRecordTypes = resolveRecordTypes(singleLevelRecordTypeKeywords)
	fourLevelRecordTypes   = resolveRecordTypes(fourLevelRecordTypeKeywords)
)

// resolveRecordTypes turns a keyword-keyed table into a UID-keyed one.
//
// It panics when a keyword is not in the UID dictionary, which can only happen
// if this package's own source is wrong: the tables are literals above, and
// every entry is checked by TestRecordTypeTablesResolve. Keeping the failure at
// init means a caller never has to handle an error the data cannot produce.
func resolveRecordTypes(byKeyword map[string]string) map[uid.UID]string {
	byUID := make(map[uid.UID]string, len(byKeyword))
	for keyword, recordType := range byKeyword {
		u, ok := uid.LookupKeyword(keyword)
		if !ok {
			panic(fmt.Sprintf("godicom: unknown SOP Class keyword %q in the Directory Record Type table", keyword))
		}
		byUID[u] = recordType
	}
	return byUID
}

// singleLevelRecordType returns the Directory Record Type for an instance that
// is filed directly under the File-set root. Anything without a single-level
// SOP Class is treated as a PATIENT, which is what puts it under the four-level
// hierarchy.
func singleLevelRecordType(ds *Dataset) string {
	if sopClass, ok := ds.GetString(tag.SOPClassUID); ok {
		if recordType, ok := singleLevelRecordTypes[uid.UID(sopClass)]; ok {
			return recordType
		}
	}
	return "PATIENT"
}

// fourLevelRecordType returns the record type of the leaf record for an
// instance filed under PATIENT/STUDY/SERIES.
//
// The Modality and attribute checks come before the SOP Class table because a
// radiotherapy plan, an encapsulated document and an RT plan instance are
// recognisable from the instance itself even when its SOP Class is a plain
// secondary capture.
func fourLevelRecordType(ds *Dataset) string {
	if modality, ok := ds.GetString(tag.Modality); ok {
		switch modality {
		case "RTINTENT", "RTSEGANN", "RTRAD":
			return "RADIOTHERAPY"
		case "PLAN":
			return "PLAN"
		}
	}
	if _, ok := ds.Get(tag.EncapsulatedDocument); ok {
		return "ENCAP DOC"
	}
	if _, ok := ds.Get(tag.RTPlanLabel); ok {
		return "RT PLAN"
	}
	if sopClass, ok := ds.GetString(tag.SOPClassUID); ok {
		if recordType, ok := fourLevelRecordTypes[uid.UID(sopClass)]; ok {
			return recordType
		}
	}
	return "IMAGE"
}

// directoryRecorders returns the record for one Directory Record Type. The
// recorders only fill in the elements PS3.3 Section F.5 asks for at that level;
// the offset elements, the record type and the file references are added by
// DirectoryRecords once the recorder has run.
var directoryRecorders = map[string]func(*Dataset) (*Dataset, error){
	"PATIENT":          definePatientRecord,
	"STUDY":            defineStudyRecord,
	"SERIES":           defineSeriesRecord,
	"IMAGE":            defineImageRecord,
	"RT DOSE":          defineRTDoseRecord,
	"RT STRUCTURE SET": defineRTStructureSetRecord,
	"RT PLAN":          defineRTPlanRecord,
	"RT TREAT RECORD":  defineRTTreatmentRecord,
	"PRESENTATION":     definePresentationRecord,
	"WAVEFORM":         defineGenericContentRecord,
	"SR DOCUMENT":      defineSRDocumentRecord,
	"KEY OBJECT DOC":   defineKeyObjectDocRecord,
	"SPECTROSCOPY":     defineSpectroscopyRecord,
	"RAW DATA":         defineGenericContentRecord,
	"REGISTRATION":     defineGenericContentIDRecord,
	"FIDUCIAL":         defineGenericContentIDRecord,
	"HANGING PROTOCOL": defineHangingProtocolRecord,
	"ENCAP DOC":        defineEncapDocRecord,
	"VALUE MAP":        defineGenericContentIDRecord,
	"STEREOMETRIC":     defineEmptyRecord,
	"PALETTE":          definePaletteRecord,
	"IMPLANT":          defineImplantRecord,
	"IMPLANT ASSY":     defineImplantAssyRecord,
	"IMPLANT GROUP":    defineImplantGroupRecord,
	"PLAN":             defineEmptyRecord,
	"MEASUREMENT":      defineGenericContentIDRecord,
	"SURFACE":          defineGenericContentIDRecord,
	"SURFACE SCAN":     defineSurfaceScanRecord,
	"TRACT":            defineGenericContentIDRecord,
	"ASSESSMENT":       defineAssessmentRecord,
	"RADIOTHERAPY":     defineRadiotherapyRecord,
}

// DirectoryRecords returns the directory records for one SOP instance, ordered
// from the highest level to the lowest. A single-level instance yields one
// record, everything else yields PATIENT, STUDY, SERIES and a leaf.
//
// The records are returned without a Referenced File ID value: that is only
// known once the instance has a place in the File-set, so the writer fills it
// in. The Referenced SOP Class, SOP Instance and Transfer Syntax UIDs are set,
// because they come from the instance itself.
//
// This is pydicom's FileSet._recordify(). It takes a dataset rather than a path
// because the caller has already read the instance. An instance that is missing
// an element PS3.3 Section F.5 requires at its level, or that has one with no
// value, is refused: filing it under a record with holes would produce a
// DICOMDIR that points at nothing usable.
func DirectoryRecords(ds *FileDataset) ([]*Dataset, error) {
	if ds == nil || ds.Dataset == nil {
		return nil, fmt.Errorf("godicom: unable to create directory records for a nil dataset")
	}

	if recordType := singleLevelRecordType(ds.Dataset); recordType != "PATIENT" {
		record, err := newDirectoryRecord(recordType, ds.Dataset)
		if err != nil {
			return nil, err
		}
		setReferencedInstance(record, ds)
		return []*Dataset{record}, nil
	}

	leafType := fourLevelRecordType(ds.Dataset)
	records := make([]*Dataset, 0, 4)
	for _, recordType := range []string{"PATIENT", "STUDY", "SERIES", leafType} {
		record, err := newDirectoryRecord(recordType, ds.Dataset)
		if err != nil {
			return nil, err
		}
		// The character set travels with the record so the values it copies are
		// decoded the same way the instance decoded them.
		if charset, ok := ds.Get(tag.SpecificCharacterSet); ok {
			record.Set(copyElement(tag.SpecificCharacterSet, charset))
		}
		records = append(records, record)
	}
	setReferencedInstance(records[len(records)-1], ds)

	return records, nil
}

// newDirectoryRecord runs the recorder for recordType and adds the elements
// every record carries regardless of its type.
func newDirectoryRecord(recordType string, ds *Dataset) (*Dataset, error) {
	recorder, ok := directoryRecorders[recordType]
	if !ok {
		return nil, fmt.Errorf("godicom: no directory recorder for record type %q", recordType)
	}

	record, err := recorder(ds)
	if err != nil {
		return nil, fmt.Errorf("godicom: unable to use the default %q record creator: %w. See DICOM PS3.3 Section F.5 and either update the instance or file it with a record of your own", recordType, err)
	}

	record.Set(NewDataElement(tag.OffsetOfTheNextDirectoryRecord, VRUL, uint32(0)))
	record.Set(NewDataElement(tag.RecordInUseFlag, VRUL, uint32(0xFFFF)))
	record.Set(NewDataElement(tag.OffsetOfReferencedLowerLevelDirectoryEntity, VRUL, uint32(0)))
	record.Set(NewDataElement(tag.DirectoryRecordType, VRCS, recordType))

	return record, nil
}

// setReferencedInstance adds the elements that name the instance the record
// points at, and an empty Referenced File ID for the writer to fill in.
func setReferencedInstance(record *Dataset, ds *FileDataset) {
	record.Set(NewDataElement(tag.ReferencedFileID, VRCS, nil))
	if sopClass, ok := ds.GetString(tag.SOPClassUID); ok {
		record.Set(NewDataElement(tag.ReferencedSOPClassUIDInFile, VRUI, sopClass))
	}
	if sopInstance, ok := ds.GetString(tag.SOPInstanceUID); ok {
		record.Set(NewDataElement(tag.ReferencedSOPInstanceUIDInFile, VRUI, sopInstance))
	}
	if ds.FileMeta != nil {
		if syntax, ok := ds.FileMeta.GetString(tag.TransferSyntaxUID); ok {
			record.Set(NewDataElement(tag.ReferencedTransferSyntaxUIDInFile, VRUI, syntax))
		}
	}
}

// checkRecordDataset refuses a dataset that is missing one of the Type 1
// elements the record needs at this level.
func checkRecordDataset(ds *Dataset, keywords ...string) error {
	for _, keyword := range keywords {
		tg, ok := tag.ByKeyword(keyword)
		if !ok {
			return fmt.Errorf("godicom: unknown element keyword %q", keyword)
		}
		name, _ := dictionaryDescription(tg)
		elem, ok := ds.Get(tg)
		if !ok {
			return fmt.Errorf("the instance's %s '%s' element is missing", tg, name)
		}
		if elem.VM() == 0 {
			return fmt.Errorf("the instance's %s '%s' element cannot be empty", tg, name)
		}
	}
	return nil
}

// copyElement returns a new element holding the same value as elem.
func copyElement(tg Tag, elem *DataElement) *DataElement {
	return NewDataElement(tg, elem.VR, elem.Value)
}

// copyRecordElement copies the element at keyword from the instance to the
// record. A missing element is an error, for the Type 1 elements.
func copyRecordElement(record, ds *Dataset, keyword string) error {
	tg, ok := tag.ByKeyword(keyword)
	if !ok {
		return fmt.Errorf("godicom: unknown element keyword %q", keyword)
	}
	elem, ok := ds.Get(tg)
	if !ok {
		return fmt.Errorf("the instance's %s element is missing", tg)
	}
	record.Set(copyElement(tg, elem))
	return nil
}

// copyRecordElementOrEmpty copies the element at keyword when the instance has
// it and adds an empty element of the dictionary's VR when it does not.
//
// pydicom reaches the same place by assigning ds.get(keyword) to the record: a
// missing element becomes a present but empty one, and the DICOMDIR then
// carries the attribute with a zero length instead of omitting it.
func copyRecordElementOrEmpty(record, ds *Dataset, keyword string) error {
	tg, ok := tag.ByKeyword(keyword)
	if !ok {
		return fmt.Errorf("godicom: unknown element keyword %q", keyword)
	}
	if elem, ok := ds.Get(tg); ok {
		record.Set(copyElement(tg, elem))
		return nil
	}
	vr, err := dictionaryVR(tg)
	if err != nil {
		return fmt.Errorf("godicom: no VR for %s: %w", tg, err)
	}
	record.Set(NewDataElement(tg, vr, nil))
	return nil
}

// sequenceElement copies a sequence element across unchanged.
func sequenceElement(record, ds *Dataset, keyword string) error {
	tg, ok := tag.ByKeyword(keyword)
	if !ok {
		return fmt.Errorf("godicom: unknown element keyword %q", keyword)
	}
	elem, ok := ds.Get(tg)
	if !ok {
		return fmt.Errorf("the instance's %s element is missing", tg)
	}
	record.Set(copyElement(tg, elem))
	return nil
}

// definePatientRecord returns a PATIENT directory record (PS3.3 Table F.5-1).
func definePatientRecord(ds *Dataset) (*Dataset, error) {
	if err := checkRecordDataset(ds, "PatientID"); err != nil {
		return nil, err
	}

	record := NewDataset()
	if err := copyRecordElementOrEmpty(record, ds, "PatientName"); err != nil {
		return nil, err
	}
	if err := copyRecordElement(record, ds, "PatientID"); err != nil {
		return nil, err
	}
	return record, nil
}

// defineStudyRecord returns a STUDY directory record (PS3.3 Table F.5-2).
func defineStudyRecord(ds *Dataset) (*Dataset, error) {
	if err := checkRecordDataset(ds, "StudyDate", "StudyTime", "StudyID"); err != nil {
		return nil, err
	}

	record := NewDataset()
	for _, keyword := range []string{"StudyDate", "StudyTime"} {
		if err := copyRecordElement(record, ds, keyword); err != nil {
			return nil, err
		}
	}
	if err := copyRecordElementOrEmpty(record, ds, "StudyDescription"); err != nil {
		return nil, err
	}
	if _, ok := ds.Get(tag.StudyInstanceUID); ok {
		if err := checkRecordDataset(ds, "StudyInstanceUID"); err != nil {
			return nil, err
		}
		if err := copyRecordElement(record, ds, "StudyInstanceUID"); err != nil {
			return nil, err
		}
	}
	if err := copyRecordElement(record, ds, "StudyID"); err != nil {
		return nil, err
	}
	if err := copyRecordElementOrEmpty(record, ds, "AccessionNumber"); err != nil {
		return nil, err
	}
	return record, nil
}

// defineSeriesRecord returns a SERIES directory record (PS3.3 Table F.5-3).
func defineSeriesRecord(ds *Dataset) (*Dataset, error) {
	if err := checkRecordDataset(ds, "Modality", "SeriesInstanceUID", "SeriesNumber"); err != nil {
		return nil, err
	}

	record := NewDataset()
	for _, keyword := range []string{"Modality", "SeriesInstanceUID", "SeriesNumber"} {
		if err := copyRecordElement(record, ds, keyword); err != nil {
			return nil, err
		}
	}
	return record, nil
}

// defineImageRecord returns an IMAGE directory record (PS3.3 Table F.5-4).
func defineImageRecord(ds *Dataset) (*Dataset, error) {
	if err := checkRecordDataset(ds, "InstanceNumber"); err != nil {
		return nil, err
	}

	record := NewDataset()
	if err := copyRecordElement(record, ds, "InstanceNumber"); err != nil {
		return nil, err
	}
	return record, nil
}

// defineRTDoseRecord returns an RT DOSE directory record.
func defineRTDoseRecord(ds *Dataset) (*Dataset, error) {
	if err := checkRecordDataset(ds, "InstanceNumber", "DoseSummationType"); err != nil {
		return nil, err
	}

	record := NewDataset()
	for _, keyword := range []string{"InstanceNumber", "DoseSummationType"} {
		if err := copyRecordElement(record, ds, keyword); err != nil {
			return nil, err
		}
	}
	return record, nil
}

// defineRTStructureSetRecord returns an RT STRUCTURE SET directory record.
func defineRTStructureSetRecord(ds *Dataset) (*Dataset, error) {
	if err := checkRecordDataset(ds, "InstanceNumber", "StructureSetLabel"); err != nil {
		return nil, err
	}

	record := NewDataset()
	for _, keyword := range []string{"InstanceNumber", "StructureSetLabel"} {
		if err := copyRecordElement(record, ds, keyword); err != nil {
			return nil, err
		}
	}
	for _, keyword := range []string{"StructureSetDate", "StructureSetTime"} {
		if err := copyRecordElementOrEmpty(record, ds, keyword); err != nil {
			return nil, err
		}
	}
	return record, nil
}

// defineRTPlanRecord returns an RT PLAN directory record.
func defineRTPlanRecord(ds *Dataset) (*Dataset, error) {
	if err := checkRecordDataset(ds, "InstanceNumber", "RTPlanLabel"); err != nil {
		return nil, err
	}

	record := NewDataset()
	for _, keyword := range []string{"InstanceNumber", "RTPlanLabel"} {
		if err := copyRecordElement(record, ds, keyword); err != nil {
			return nil, err
		}
	}
	for _, keyword := range []string{"RTPlanDate", "RTPlanTime"} {
		if err := copyRecordElementOrEmpty(record, ds, keyword); err != nil {
			return nil, err
		}
	}
	return record, nil
}

// defineRTTreatmentRecord returns an RT TREAT RECORD directory record.
func defineRTTreatmentRecord(ds *Dataset) (*Dataset, error) {
	if err := checkRecordDataset(ds, "InstanceNumber"); err != nil {
		return nil, err
	}

	record := NewDataset()
	if err := copyRecordElement(record, ds, "InstanceNumber"); err != nil {
		return nil, err
	}
	for _, keyword := range []string{"TreatmentDate", "TreatmentTime"} {
		if err := copyRecordElementOrEmpty(record, ds, keyword); err != nil {
			return nil, err
		}
	}
	return record, nil
}

// definePresentationRecord returns a PRESENTATION directory record.
func definePresentationRecord(ds *Dataset) (*Dataset, error) {
	if err := checkRecordDataset(ds,
		"PresentationCreationDate",
		"PresentationCreationTime",
		"InstanceNumber",
		"ContentLabel",
	); err != nil {
		return nil, err
	}

	record := NewDataset()
	for _, keyword := range []string{
		"PresentationCreationDate",
		"PresentationCreationTime",
		// Content Identification Macro
		"InstanceNumber",
		"ContentLabel",
	} {
		if err := copyRecordElement(record, ds, keyword); err != nil {
			return nil, err
		}
	}
	for _, keyword := range []string{"ContentDescription", "ContentCreatorName"} {
		if err := copyRecordElementOrEmpty(record, ds, keyword); err != nil {
			return nil, err
		}
	}
	for _, keyword := range []string{"ReferencedSeriesSequence", "BlendingSequence"} {
		if _, ok := ds.Get(mustTagForKeyword(keyword)); ok {
			if err := checkRecordDataset(ds, keyword); err != nil {
				return nil, err
			}
			if err := sequenceElement(record, ds, keyword); err != nil {
				return nil, err
			}
		}
	}
	return record, nil
}

// defineSRDocumentRecord returns an SR DOCUMENT directory record.
func defineSRDocumentRecord(ds *Dataset) (*Dataset, error) {
	if err := checkRecordDataset(ds,
		"InstanceNumber",
		"CompletionFlag",
		"VerificationFlag",
		"ContentDate",
		"ContentTime",
		"ConceptNameCodeSequence",
	); err != nil {
		return nil, err
	}

	record := NewDataset()
	for _, keyword := range []string{
		"InstanceNumber",
		"CompletionFlag",
		"VerificationFlag",
		"ContentDate",
		"ContentTime",
	} {
		if err := copyRecordElement(record, ds, keyword); err != nil {
			return nil, err
		}
	}
	if _, ok := ds.Get(tag.VerificationDateTime); ok {
		if err := checkRecordDataset(ds, "VerificationDateTime"); err != nil {
			return nil, err
		}
		if err := copyRecordElement(record, ds, "VerificationDateTime"); err != nil {
			return nil, err
		}
	}
	if err := sequenceElement(record, ds, "ConceptNameCodeSequence"); err != nil {
		return nil, err
	}
	if _, ok := ds.Get(tag.ContentSequence); ok {
		if err := checkRecordDataset(ds, "ContentSequence"); err != nil {
			return nil, err
		}
		if err := sequenceElement(record, ds, "ContentSequence"); err != nil {
			return nil, err
		}
	}
	return record, nil
}

// defineKeyObjectDocRecord returns a KEY OBJECT DOC directory record.
func defineKeyObjectDocRecord(ds *Dataset) (*Dataset, error) {
	if err := checkRecordDataset(ds,
		"InstanceNumber",
		"ContentDate",
		"ContentTime",
		"ConceptNameCodeSequence",
	); err != nil {
		return nil, err
	}

	record := NewDataset()
	for _, keyword := range []string{"ContentDate", "ContentTime", "InstanceNumber"} {
		if err := copyRecordElement(record, ds, keyword); err != nil {
			return nil, err
		}
	}
	if err := sequenceElement(record, ds, "ConceptNameCodeSequence"); err != nil {
		return nil, err
	}
	if _, ok := ds.Get(tag.ContentSequence); ok {
		if err := checkRecordDataset(ds, "ContentSequence"); err != nil {
			return nil, err
		}
		if err := sequenceElement(record, ds, "ContentSequence"); err != nil {
			return nil, err
		}
	}
	return record, nil
}

// defineSpectroscopyRecord returns a SPECTROSCOPY directory record.
func defineSpectroscopyRecord(ds *Dataset) (*Dataset, error) {
	if err := checkRecordDataset(ds,
		"ImageType",
		"ContentDate",
		"ContentTime",
		"InstanceNumber",
		"NumberOfFrames",
		"Rows",
		"Columns",
		"DataPointRows",
		"DataPointColumns",
	); err != nil {
		return nil, err
	}

	record := NewDataset()
	for _, keyword := range []string{
		"ImageType",
		"ContentDate",
		"ContentTime",
		"InstanceNumber",
	} {
		if err := copyRecordElement(record, ds, keyword); err != nil {
			return nil, err
		}
	}
	if _, ok := ds.Get(tag.ReferencedImageEvidenceSequence); ok {
		if err := checkRecordDataset(ds, "ReferencedImageEvidenceSequence"); err != nil {
			return nil, err
		}
		if err := sequenceElement(record, ds, "ReferencedImageEvidenceSequence"); err != nil {
			return nil, err
		}
	}
	for _, keyword := range []string{
		"NumberOfFrames",
		"Rows",
		"Columns",
		"DataPointRows",
		"DataPointColumns",
	} {
		if err := copyRecordElement(record, ds, keyword); err != nil {
			return nil, err
		}
	}
	return record, nil
}

// defineHangingProtocolRecord returns a HANGING PROTOCOL directory record.
func defineHangingProtocolRecord(ds *Dataset) (*Dataset, error) {
	if err := checkRecordDataset(ds,
		"HangingProtocolCreator",
		"HangingProtocolCreationDateTime",
		"HangingProtocolDefinitionSequence",
		"NumberOfPriorsReferenced",
	); err != nil {
		return nil, err
	}

	record := NewDataset()
	if err := copyRecordElement(record, ds, "HangingProtocolCreator"); err != nil {
		return nil, err
	}
	if err := copyRecordElement(record, ds, "HangingProtocolCreationDateTime"); err != nil {
		return nil, err
	}
	if err := sequenceElement(record, ds, "HangingProtocolDefinitionSequence"); err != nil {
		return nil, err
	}
	if err := copyRecordElement(record, ds, "NumberOfPriorsReferenced"); err != nil {
		return nil, err
	}
	// pydicom defaults this to an empty list, so the element is present and
	// empty rather than absent.
	if err := copyRecordElementOrEmpty(record, ds, "HangingProtocolUserIdentificationCodeSequence"); err != nil {
		return nil, err
	}
	return record, nil
}

// defineEncapDocRecord returns an ENCAP DOC directory record.
func defineEncapDocRecord(ds *Dataset) (*Dataset, error) {
	if err := checkRecordDataset(ds, "InstanceNumber", "MIMETypeOfEncapsulatedDocument"); err != nil {
		return nil, err
	}

	record := NewDataset()
	for _, keyword := range []string{"ContentDate", "ContentTime"} {
		if err := copyRecordElementOrEmpty(record, ds, keyword); err != nil {
			return nil, err
		}
	}
	if err := copyRecordElement(record, ds, "InstanceNumber"); err != nil {
		return nil, err
	}
	if err := copyRecordElementOrEmpty(record, ds, "DocumentTitle"); err != nil {
		return nil, err
	}
	if _, ok := ds.Get(tag.HL7InstanceIdentifier); ok {
		if err := checkRecordDataset(ds, "HL7InstanceIdentifier"); err != nil {
			return nil, err
		}
		if err := copyRecordElement(record, ds, "HL7InstanceIdentifier"); err != nil {
			return nil, err
		}
	}
	if err := copyRecordElementOrEmpty(record, ds, "ConceptNameCodeSequence"); err != nil {
		return nil, err
	}
	if err := copyRecordElement(record, ds, "MIMETypeOfEncapsulatedDocument"); err != nil {
		return nil, err
	}
	return record, nil
}

// definePaletteRecord returns a PALETTE directory record.
func definePaletteRecord(ds *Dataset) (*Dataset, error) {
	if err := checkRecordDataset(ds, "ContentLabel"); err != nil {
		return nil, err
	}

	record := NewDataset()
	if err := copyRecordElement(record, ds, "ContentLabel"); err != nil {
		return nil, err
	}
	if err := copyRecordElementOrEmpty(record, ds, "ContentDescription"); err != nil {
		return nil, err
	}
	return record, nil
}

// defineImplantRecord returns an IMPLANT directory record.
func defineImplantRecord(ds *Dataset) (*Dataset, error) {
	if err := checkRecordDataset(ds, "Manufacturer", "ImplantName", "ImplantPartNumber"); err != nil {
		return nil, err
	}

	record := NewDataset()
	for _, keyword := range []string{"Manufacturer", "ImplantName"} {
		if err := copyRecordElement(record, ds, keyword); err != nil {
			return nil, err
		}
	}
	if _, ok := ds.Get(tag.ImplantSize); ok {
		if err := checkRecordDataset(ds, "ImplantSize"); err != nil {
			return nil, err
		}
		if err := copyRecordElement(record, ds, "ImplantSize"); err != nil {
			return nil, err
		}
	}
	if err := copyRecordElement(record, ds, "ImplantPartNumber"); err != nil {
		return nil, err
	}
	return record, nil
}

// defineImplantAssyRecord returns an IMPLANT ASSY directory record.
func defineImplantAssyRecord(ds *Dataset) (*Dataset, error) {
	if err := checkRecordDataset(ds,
		"ImplantAssemblyTemplateName",
		"Manufacturer",
		"ProcedureTypeCodeSequence",
	); err != nil {
		return nil, err
	}

	record := NewDataset()
	if err := copyRecordElement(record, ds, "ImplantAssemblyTemplateName"); err != nil {
		return nil, err
	}
	if err := copyRecordElement(record, ds, "Manufacturer"); err != nil {
		return nil, err
	}
	if err := sequenceElement(record, ds, "ProcedureTypeCodeSequence"); err != nil {
		return nil, err
	}
	return record, nil
}

// defineImplantGroupRecord returns an IMPLANT GROUP directory record.
func defineImplantGroupRecord(ds *Dataset) (*Dataset, error) {
	if err := checkRecordDataset(ds, "ImplantTemplateGroupName", "ImplantTemplateGroupIssuer"); err != nil {
		return nil, err
	}

	record := NewDataset()
	for _, keyword := range []string{"ImplantTemplateGroupName", "ImplantTemplateGroupIssuer"} {
		if err := copyRecordElement(record, ds, keyword); err != nil {
			return nil, err
		}
	}
	return record, nil
}

// defineSurfaceScanRecord returns a SURFACE SCAN directory record.
func defineSurfaceScanRecord(ds *Dataset) (*Dataset, error) {
	if err := checkRecordDataset(ds, "ContentDate", "ContentTime"); err != nil {
		return nil, err
	}

	record := NewDataset()
	for _, keyword := range []string{"ContentDate", "ContentTime"} {
		if err := copyRecordElement(record, ds, keyword); err != nil {
			return nil, err
		}
	}
	return record, nil
}

// defineAssessmentRecord returns an ASSESSMENT directory record.
func defineAssessmentRecord(ds *Dataset) (*Dataset, error) {
	if err := checkRecordDataset(ds, "InstanceNumber", "InstanceCreationDate"); err != nil {
		return nil, err
	}

	record := NewDataset()
	if err := copyRecordElement(record, ds, "InstanceNumber"); err != nil {
		return nil, err
	}
	if err := copyRecordElement(record, ds, "InstanceCreationDate"); err != nil {
		return nil, err
	}
	if err := copyRecordElementOrEmpty(record, ds, "InstanceCreationTime"); err != nil {
		return nil, err
	}
	return record, nil
}

// defineRadiotherapyRecord returns a RADIOTHERAPY directory record.
func defineRadiotherapyRecord(ds *Dataset) (*Dataset, error) {
	if err := checkRecordDataset(ds, "InstanceNumber"); err != nil {
		return nil, err
	}

	record := NewDataset()
	if err := copyRecordElement(record, ds, "InstanceNumber"); err != nil {
		return nil, err
	}
	for _, keyword := range []string{"UserContentLabel", "UserContentLongLabel"} {
		if _, ok := ds.Get(mustTagForKeyword(keyword)); ok {
			if err := checkRecordDataset(ds, keyword); err != nil {
				return nil, err
			}
			if err := copyRecordElement(record, ds, keyword); err != nil {
				return nil, err
			}
		}
	}
	for _, keyword := range []string{"ContentDescription", "ContentCreatorName"} {
		if err := copyRecordElementOrEmpty(record, ds, keyword); err != nil {
			return nil, err
		}
	}
	return record, nil
}

// defineGenericContentRecord returns a WAVEFORM or RAW DATA directory record.
func defineGenericContentRecord(ds *Dataset) (*Dataset, error) {
	if err := checkRecordDataset(ds, "InstanceNumber", "ContentDate", "ContentTime"); err != nil {
		return nil, err
	}

	record := NewDataset()
	for _, keyword := range []string{"InstanceNumber", "ContentDate", "ContentTime"} {
		if err := copyRecordElement(record, ds, keyword); err != nil {
			return nil, err
		}
	}
	return record, nil
}

// defineGenericContentIDRecord returns a directory record carrying the Content
// Identification Macro: REGISTRATION, FIDUCIAL, VALUE MAP, MEASUREMENT,
// SURFACE and TRACT all use it.
func defineGenericContentIDRecord(ds *Dataset) (*Dataset, error) {
	if err := checkRecordDataset(ds, "InstanceNumber", "ContentDate", "ContentTime", "ContentLabel"); err != nil {
		return nil, err
	}

	record := NewDataset()
	for _, keyword := range []string{"InstanceNumber", "ContentDate", "ContentTime", "ContentLabel"} {
		if err := copyRecordElement(record, ds, keyword); err != nil {
			return nil, err
		}
	}
	for _, keyword := range []string{"ContentDescription", "ContentCreatorName"} {
		if err := copyRecordElementOrEmpty(record, ds, keyword); err != nil {
			return nil, err
		}
	}
	return record, nil
}

// defineEmptyRecord returns a directory record with nothing but the elements
// every record carries. STEREOMETRIC and PLAN define no attributes of their own.
func defineEmptyRecord(ds *Dataset) (*Dataset, error) {
	return NewDataset(), nil
}

// mustTagForKeyword returns the tag for a DICOM keyword.
//
// It panics on an unknown keyword, which can only come from a literal in this
// file; TestRecordTypeTablesResolve walks the same literals so a typo fails in
// the test suite rather than at run time.
func mustTagForKeyword(keyword string) Tag {
	tg, ok := tag.ByKeyword(keyword)
	if !ok {
		panic(fmt.Sprintf("godicom: unknown element keyword %q", keyword))
	}
	return tg
}
