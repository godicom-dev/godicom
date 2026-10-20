package godicom

import (
	"embed"
	"encoding/binary"
	"fmt"
	"strings"

	"github.com/godicom-dev/godicom/pixels"
	"github.com/godicom-dev/godicom/tag"
)

// The eight DICOM well-known color palettes, PS3.6 Annex B. pydicom ships the
// same files and apply_color_lut loads them by name or SOP Instance UID.
//
//go:embed palettes/*.dcm
var wellKnownPaletteFS embed.FS

// wellKnownPaletteUID maps a content label to its SOP Instance UID.
// pydicom.pixels.processing.apply_color_lut swaps the FALL and WINTER names
// relative to this table and to PS3.6; the UID-to-file map in that function
// does not. Names here follow the standard.
var wellKnownPaletteUID = map[string]string{
	"HOT_IRON":       "1.2.840.10008.1.5.1",
	"PET":            "1.2.840.10008.1.5.2",
	"HOT_METAL_BLUE": "1.2.840.10008.1.5.3",
	"PET_20_STEP":    "1.2.840.10008.1.5.4",
	"SPRING":         "1.2.840.10008.1.5.5",
	"SUMMER":         "1.2.840.10008.1.5.6",
	"FALL":           "1.2.840.10008.1.5.7",
	"WINTER":         "1.2.840.10008.1.5.8",
}

var wellKnownPaletteFile = map[string]string{
	"1.2.840.10008.1.5.1": "hotiron.dcm",
	"1.2.840.10008.1.5.2": "pet.dcm",
	"1.2.840.10008.1.5.3": "hotmetalblue.dcm",
	"1.2.840.10008.1.5.4": "pet20step.dcm",
	"1.2.840.10008.1.5.5": "spring.dcm",
	"1.2.840.10008.1.5.6": "summer.dcm",
	"1.2.840.10008.1.5.7": "fall.dcm",
	"1.2.840.10008.1.5.8": "winter.dcm",
}

// ApplyColorLUT maps stored pixel values through a palette color lookup table.
// palette is a well-known content label (HOT_IRON, PET, ...) or its SOP Instance
// UID. When palette is set it supplies the table, which is what pydicom does
// when both a dataset and a palette are passed. When palette is empty, ds must
// carry a Palette Color Lookup Table Module.
//
// Enhanced and supplemental palette modules are not supported.
// Mirrors pydicom.pixels.processing.apply_color_lut.
func ApplyColorLUT(arr []float64, ds *FileDataset, palette string) (pixels.PalettePixels, error) {
	if ds == nil && palette == "" {
		return pixels.PalettePixels{}, fmt.Errorf("godicom: either a dataset or a palette is required")
	}
	if palette != "" {
		loaded, err := readWellKnownPalette(palette)
		if err != nil {
			return pixels.PalettePixels{}, err
		}
		ds = loaded
	}
	lut, err := ds.colorLUT()
	if err != nil {
		return pixels.PalettePixels{}, err
	}
	out, err := pixels.ApplyColorLUT(arr, lut)
	if err != nil {
		return pixels.PalettePixels{}, err
	}
	return out, nil
}

func readWellKnownPalette(nameOrUID string) (*FileDataset, error) {
	uid := nameOrUID
	if !looksLikeUID(nameOrUID) {
		mapped, ok := wellKnownPaletteUID[nameOrUID]
		if !ok {
			return nil, fmt.Errorf("godicom: unknown palette '%s'", nameOrUID)
		}
		uid = mapped
	}
	file, ok := wellKnownPaletteFile[uid]
	if !ok {
		return nil, fmt.Errorf("godicom: unknown palette '%s'", nameOrUID)
	}
	b, err := wellKnownPaletteFS.ReadFile("palettes/" + file)
	if err != nil {
		return nil, fmt.Errorf("godicom: unknown palette '%s'", nameOrUID)
	}
	ds, err := ReadBytes(b, nil)
	if err != nil {
		return nil, err
	}
	return ds, nil
}

func looksLikeUID(s string) bool {
	if s == "" || !strings.Contains(s, ".") {
		return false
	}
	for _, r := range s {
		if r != '.' && (r < '0' || r > '9') {
			return false
		}
	}
	return true
}

func (fd *FileDataset) colorLUT() (pixels.ColorLUT, error) {
	if fd == nil || fd.Dataset == nil {
		return pixels.ColorLUT{}, fmt.Errorf("godicom: either a dataset or a palette is required")
	}
	if pres, ok := fd.GetString(tag.PixelPresentation); ok {
		switch strings.ToUpper(strings.TrimSpace(pres)) {
		case "MIXED", "COLOR":
			return pixels.ColorLUT{}, fmt.Errorf("godicom: use of this function with the Supplemental Palette Color Lookup Table Module is not currently supported")
		}
	}
	descElem, ok := fd.Get(tag.RedPaletteColorLookupTableDescriptor)
	if !ok || descElem == nil {
		return pixels.ColorLUT{}, fmt.Errorf("godicom: No suitable Palette Color Lookup Table Module found")
	}
	desc, ok := intSliceFromValue(descElem.Value)
	if !ok || len(desc) < 3 {
		return pixels.ColorLUT{}, fmt.Errorf("godicom: No suitable Palette Color Lookup Table Module found")
	}
	nrEntries := desc[0]
	if nrEntries == 0 {
		nrEntries = 1 << 16
	}
	firstMap := desc[1]
	nominal := desc[2]
	if nominal%8 != 0 {
		return pixels.ColorLUT{}, fmt.Errorf("godicom: the size of the LUT entries must be 8 or 16-bit, not %d-bit", nominal)
	}

	if raw, ok := fd.paletteBytes(tag.RedPaletteColorLookupTableData); ok {
		return fd.directColorLUT(raw, nrEntries, firstMap, nominal)
	}
	if raw, ok := fd.paletteBytes(tag.SegmentedRedPaletteColorLookupTableData); ok {
		return fd.segmentedColorLUT(raw, nrEntries, firstMap, nominal)
	}
	return pixels.ColorLUT{}, fmt.Errorf("godicom: No suitable Palette Color Lookup Table Module found")
}

func (fd *FileDataset) directColorLUT(red []byte, nrEntries, firstMap, nominal int) (pixels.ColorLUT, error) {
	actual := float64(len(red)) / float64(nrEntries) * 8
	if actual != 8 && actual != 16 {
		return pixels.ColorLUT{}, fmt.Errorf("godicom: the bit depth of the LUT data '%.1f' is invalid (only 8 or 16 bits per entry allowed)", actual)
	}
	byteDepth := int(actual) / 8
	channels := []struct {
		tg   tag.Tag
		need bool
	}{
		{tag.RedPaletteColorLookupTableData, true},
		{tag.GreenPaletteColorLookupTableData, true},
		{tag.BluePaletteColorLookupTableData, true},
		{tag.AlphaPaletteColorLookupTableData, false},
	}
	tables := make([][]uint16, 0, 4)
	for _, ch := range channels {
		raw, ok := fd.paletteBytes(ch.tg)
		if !ok {
			if ch.need {
				return pixels.ColorLUT{}, fmt.Errorf("godicom: No suitable Palette Color Lookup Table Module found")
			}
			continue
		}
		tables = append(tables, unpackPaletteBytes(raw, byteDepth))
	}
	if !sameLUTLength(tables) {
		return pixels.ColorLUT{}, fmt.Errorf("godicom: LUT data must be the same length")
	}
	_ = nominal
	return pixels.ColorLUT{
		FirstMap:  firstMap,
		NrEntries: nrEntries,
		Bits:      int(actual),
		Tables:    tables,
	}, nil
}

func (fd *FileDataset) segmentedColorLUT(red []byte, nrEntries, firstMap, nominal int) (pixels.ColorLUT, error) {
	if nominal != 8 && nominal != 16 {
		return pixels.ColorLUT{}, fmt.Errorf("godicom: the bit depth of the LUT data '%.1f' is invalid (only 8 or 16 bits per entry allowed)", float64(nominal))
	}
	little, err := fd.segmentEndian()
	if err != nil {
		return pixels.ColorLUT{}, err
	}
	channels := []struct {
		tg   tag.Tag
		need bool
	}{
		{tag.SegmentedRedPaletteColorLookupTableData, true},
		{tag.SegmentedGreenPaletteColorLookupTableData, true},
		{tag.SegmentedBluePaletteColorLookupTableData, true},
		{tag.SegmentedAlphaPaletteColorLookupTableData, false},
	}
	tables := make([][]uint16, 0, 4)
	for _, ch := range channels {
		raw, ok := fd.paletteBytes(ch.tg)
		if !ok {
			if ch.need {
				return pixels.ColorLUT{}, fmt.Errorf("godicom: No suitable Palette Color Lookup Table Module found")
			}
			continue
		}
		words, err := unpackSegmentWords(raw, nominal, little)
		if err != nil {
			return pixels.ColorLUT{}, err
		}
		expanded, err := pixels.ExpandSegmentedLUT(words, nominal, little, 0, 0)
		if err != nil {
			return pixels.ColorLUT{}, err
		}
		table := make([]uint16, len(expanded))
		for i, v := range expanded {
			table[i] = uint16(v)
		}
		tables = append(tables, table)
	}
	if !sameLUTLength(tables) {
		return pixels.ColorLUT{}, fmt.Errorf("godicom: LUT data must be the same length")
	}
	_ = red
	return pixels.ColorLUT{
		FirstMap:  firstMap,
		NrEntries: nrEntries,
		Bits:      nominal,
		Tables:    tables,
	}, nil
}

func (fd *FileDataset) segmentEndian() (bool, error) {
	if fd.FileMeta != nil {
		if ts, ok := fd.TransferSyntaxUID(); ok && ts != "" {
			return ts.IsLittleEndian(), nil
		}
	}
	if fd.Dataset == nil {
		return false, fmt.Errorf("godicom: unable to determine the endianness of the dataset, please set an appropriate Transfer Syntax UID in FileDataset.FileMeta")
	}
	return fd.originalEnc.IsLittleEndian, nil
}

func (fd *FileDataset) paletteBytes(tg tag.Tag) ([]byte, bool) {
	elem, ok := fd.Get(tg)
	if !ok || elem == nil {
		return nil, false
	}
	b, ok := elem.Value.([]byte)
	if !ok || len(b) == 0 {
		return nil, false
	}
	return b, true
}

func unpackPaletteBytes(b []byte, byteDepth int) []uint16 {
	n := len(b) / byteDepth
	out := make([]uint16, n)
	if byteDepth == 1 {
		for i := 0; i < n; i++ {
			out[i] = uint16(b[i])
		}
		return out
	}
	for i := 0; i < n; i++ {
		out[i] = binary.LittleEndian.Uint16(b[i*2 : i*2+2])
	}
	return out
}

func unpackSegmentWords(b []byte, bits int, little bool) ([]int, error) {
	if bits == 8 {
		out := make([]int, len(b))
		for i, v := range b {
			out[i] = int(v)
		}
		return out, nil
	}
	if len(b)%2 != 0 {
		return nil, fmt.Errorf("godicom: segmented palette LUT data length %d is not a multiple of 2", len(b))
	}
	order := binary.ByteOrder(binary.LittleEndian)
	if !little {
		order = binary.BigEndian
	}
	out := make([]int, len(b)/2)
	for i := range out {
		out[i] = int(order.Uint16(b[i*2 : i*2+2]))
	}
	return out, nil
}

func sameLUTLength(tables [][]uint16) bool {
	if len(tables) == 0 {
		return false
	}
	n := len(tables[0])
	for _, table := range tables[1:] {
		if len(table) != n {
			return false
		}
	}
	return true
}

func intSliceFromValue(v interface{}) ([]int, bool) {
	switch x := v.(type) {
	case []int:
		return x, true
	case []int64:
		return intsFrom64(x), true
	case []uint64:
		return intsFromU64(x), true
	case []uint16:
		out := make([]int, len(x))
		for i, n := range x {
			out[i] = int(n)
		}
		return out, true
	case *MultiValue[int]:
		if x == nil {
			return nil, false
		}
		return append([]int(nil), x.Values()...), true
	case *MultiValue[int64]:
		if x == nil {
			return nil, false
		}
		return intsFrom64(x.Values()), true
	case *MultiValue[uint64]:
		if x == nil {
			return nil, false
		}
		return intsFromU64(x.Values()), true
	case *MultiValue[uint16]:
		if x == nil {
			return nil, false
		}
		vals := x.Values()
		out := make([]int, len(vals))
		for i, n := range vals {
			out[i] = int(n)
		}
		return out, true
	case int:
		return []int{x}, true
	case int64:
		return []int{int(x)}, true
	case uint64:
		return []int{int(x)}, true
	case uint16:
		return []int{int(x)}, true
	default:
		return nil, false
	}
}

func intsFrom64(v []int64) []int {
	out := make([]int, len(v))
	for i, n := range v {
		out[i] = int(n)
	}
	return out
}

func intsFromU64(v []uint64) []int {
	out := make([]int, len(v))
	for i, n := range v {
		out[i] = int(n)
	}
	return out
}
