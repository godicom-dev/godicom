package dicomjson

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"

	"github.com/godicom-dev/godicom"
)

// DecodeDataset reads a DICOM JSON Model dataset from r.
func DecodeDataset(r io.Reader, opts ...Option) (*godicom.Dataset, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	return ParseDataset(data, opts...)
}

// ParseDataset parses a DICOM JSON Model dataset.
func ParseDataset(data []byte, opts ...Option) (*godicom.Dataset, error) {
	var raw map[string]rawElement
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	return parseDatasetMap(raw, applyOptions(opts))
}

func parseDatasetMap(raw map[string]rawElement, opts options) (*godicom.Dataset, error) {
	ds := godicom.NewDataset()
	keys := make([]string, 0, len(raw))
	for key := range raw {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		elem, err := parseElement(key, raw[key], opts)
		if err != nil {
			return nil, err
		}
		ds.Set(elem)
	}
	return ds, nil
}

func parseElement(key string, raw rawElement, opts options) (*godicom.DataElement, error) {
	tagValue, err := strconv.ParseUint(key, 16, 32)
	if err != nil {
		return nil, fmt.Errorf("dicomjson: data element %q could not be loaded from JSON: %w", key, err)
	}
	tag := godicom.Tag(tagValue)
	vr := godicom.VR(raw.VR)

	valueKey, err := jsonValueKey(raw)
	if err != nil {
		return nil, err
	}
	if valueKey == "" {
		return godicom.NewDataElement(tag, vr, emptyJSONValue(vr)), nil
	}

	switch valueKey {
	case "Value":
		return parseRegularElement(tag, vr, raw.Value, opts)
	case "InlineBinary":
		return parseInlineBinaryElement(tag, vr, raw.InlineBinary)
	case "BulkDataURI":
		return parseBulkDataURIElement(tag, vr, raw.BulkDataURI, opts)
	default:
		return nil, fmt.Errorf("dicomjson: unknown value key %q for tag %s", valueKey, tag)
	}
}

func jsonValueKey(raw rawElement) (string, error) {
	keys := make([]string, 0, 3)
	if raw.Value != nil {
		keys = append(keys, "Value")
	}
	if raw.InlineBinary != nil {
		keys = append(keys, "InlineBinary")
	}
	if raw.BulkDataURI != nil {
		keys = append(keys, "BulkDataURI")
	}
	if len(keys) > 1 {
		return "", fmt.Errorf("dicomjson: data element has multiple value keys: %v", keys)
	}
	if len(keys) == 0 {
		return "", nil
	}
	return keys[0], nil
}

func parseRegularElement(
	tag godicom.Tag,
	vr godicom.VR,
	data json.RawMessage,
	opts options,
) (*godicom.DataElement, error) {
	var values []json.RawMessage
	if err := json.Unmarshal(data, &values); err != nil {
		return nil, fmt.Errorf("dicomjson: Value of data element %s must be a list: %w", tag, err)
	}
	if len(values) == 0 {
		return godicom.NewDataElement(tag, vr, emptyJSONValue(vr)), nil
	}

	if vr == godicom.VRSQ {
		items := make([]*godicom.Dataset, 0, len(values))
		for _, itemData := range values {
			if string(itemData) == "null" {
				items = append(items, godicom.NewDataset())
				continue
			}
			var item map[string]rawElement
			if err := json.Unmarshal(itemData, &item); err != nil {
				return nil, fmt.Errorf("dicomjson: SQ item for %s must be an object: %w", tag, err)
			}
			ds, err := parseDatasetMap(item, opts)
			if err != nil {
				return nil, err
			}
			items = append(items, ds)
		}
		return godicom.NewDataElement(tag, vr, godicom.NewSequence(items)), nil
	}

	parsed := make([]interface{}, 0, len(values))
	for _, item := range values {
		value, err := parseValue(tag, vr, item)
		if err != nil {
			return nil, err
		}
		coerced, err := coerceParsedValue(vr, value)
		if err != nil {
			return nil, err
		}
		parsed = append(parsed, coerced)
	}

	if len(parsed) == 1 {
		return godicom.NewDataElement(tag, vr, parsed[0]), nil
	}
	return godicom.NewDataElement(tag, vr, godicom.NewMultiValue(parsed)), nil
}

func parseValue(tag godicom.Tag, vr godicom.VR, data json.RawMessage) (interface{}, error) {
	if string(data) == "null" {
		return emptyJSONValue(vr), nil
	}

	switch vr {
	case godicom.VRPN:
		var comps map[string]string
		if err := json.Unmarshal(data, &comps); err != nil {
			var s string
			if stringErr := json.Unmarshal(data, &s); stringErr == nil {
				return godicom.ParsePersonName(s), nil
			}
			return nil, fmt.Errorf("dicomjson: PN value for %s must be an object: %w", tag, err)
		}
		return personNameFromComponents(comps), nil
	case godicom.VRAT:
		var s string
		if err := json.Unmarshal(data, &s); err != nil {
			return nil, fmt.Errorf("dicomjson: AT value for %s must be a string: %w", tag, err)
		}
		if s == "" {
			return nil, nil
		}
		value, err := strconv.ParseUint(s, 16, 32)
		if err != nil {
			return nil, nil
		}
		return godicom.Tag(value), nil
	}

	if isIntegerVR(vr) {
		v, err := parseJSONInt(data)
		if err != nil {
			return nil, err
		}
		return coerceParsedValue(vr, v)
	}
	if isFloatVR(vr) {
		v, err := parseJSONFloat(data)
		if err != nil {
			return nil, err
		}
		return coerceParsedValue(vr, v)
	}

	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		if s == "" {
			return emptyJSONValue(vr), nil
		}
		return coerceParsedValue(vr, s)
	}

	var raw interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	return raw, nil
}

func parseInlineBinaryElement(tag godicom.Tag, vr godicom.VR, data json.RawMessage) (*godicom.DataElement, error) {
	s, err := stringOrFirstString(data, "InlineBinary", tag)
	if err != nil {
		return nil, err
	}
	value, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("dicomjson: invalid InlineBinary for %s: %w", tag, err)
	}
	if vr == godicom.VRUN {
		if elem, ok, err := parseUnknownInlineBinary(tag, value); err != nil || ok {
			return elem, err
		}
	}
	return godicom.NewDataElement(tag, vr, value), nil
}

func parseBulkDataURIElement(
	tag godicom.Tag,
	vr godicom.VR,
	data json.RawMessage,
	opts options,
) (*godicom.DataElement, error) {
	uri, err := stringOrFirstString(data, "BulkDataURI", tag)
	if err != nil {
		return nil, err
	}
	if opts.bulkDataURIReader == nil {
		return godicom.NewDataElement(tag, vr, emptyJSONValue(vr)), nil
	}
	value, err := opts.bulkDataURIReader(tag, vr, uri)
	if err != nil {
		return nil, err
	}
	return godicom.NewDataElement(tag, vr, value), nil
}

func stringOrFirstString(data json.RawMessage, key string, tag godicom.Tag) (string, error) {
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		return s, nil
	}
	var values []string
	if err := json.Unmarshal(data, &values); err != nil || len(values) == 0 {
		return "", fmt.Errorf("dicomjson: %s for %s must be a string", key, tag)
	}
	return values[0], nil
}

func personNameFromComponents(comps map[string]string) godicom.PersonName {
	return godicom.PersonName{
		Alphabetic:  comps["Alphabetic"],
		Ideographic: comps["Ideographic"],
		Phonetic:    comps["Phonetic"],
	}
}

func parseJSONInt(data json.RawMessage) (interface{}, error) {
	if string(data) == "\"\"" {
		return nil, nil
	}
	var i int64
	if err := json.Unmarshal(data, &i); err == nil {
		return i, nil
	}
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	if s == "" {
		return nil, nil
	}
	return strconv.ParseInt(s, 10, 64)
}

func parseJSONFloat(data json.RawMessage) (interface{}, error) {
	if string(data) == "\"\"" {
		return nil, nil
	}
	var f float64
	if err := json.Unmarshal(data, &f); err == nil {
		return f, nil
	}
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	if s == "" {
		return nil, nil
	}
	return strconv.ParseFloat(s, 64)
}
