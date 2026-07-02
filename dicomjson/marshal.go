package dicomjson

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/godicom-dev/godicom"
)

// MarshalDatasetString returns a sorted JSON string for a DICOM JSON Model dataset.
func MarshalDatasetString(ds *godicom.Dataset, opts ...Option) (string, error) {
	data, err := MarshalDataset(ds, opts...)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// MarshalDataset returns sorted JSON for a DICOM JSON Model dataset.
func MarshalDataset(ds *godicom.Dataset, opts ...Option) ([]byte, error) {
	m, err := DatasetToMap(ds, opts...)
	if err != nil {
		return nil, err
	}
	return json.Marshal(m)
}

// DatasetToMap returns the DICOM JSON Model map representation of ds.
func DatasetToMap(ds *godicom.Dataset, opts ...Option) (map[string]Element, error) {
	if ds == nil {
		return map[string]Element{}, nil
	}
	return datasetToMap(ds, applyOptions(opts))
}

func datasetToMap(ds *godicom.Dataset, opts options) (map[string]Element, error) {
	out := make(map[string]Element, ds.Len())
	for _, tag := range ds.SortedTags() {
		elem, ok := ds.Get(tag)
		if !ok {
			continue
		}
		jsonElem, err := elementToJSON(elem, opts)
		if err != nil {
			if opts.suppressInvalidTags {
				continue
			}
			return nil, err
		}
		out[tag.JSONKey()] = jsonElem
	}
	return out, nil
}

func elementToJSON(elem *godicom.DataElement, opts options) (Element, error) {
	out := Element{VR: string(elem.VR)}
	if elem.Value == nil || elem.IsEmpty() {
		return out, nil
	}

	if isBinaryVR(elem.VR) {
		value := binaryValue(elem)
		if len(value) == 0 {
			return out, nil
		}
		threshold := (opts.bulkDataThreshold / 4) * 3
		if opts.bulkDataURIBuilder != nil && len(value) > threshold {
			uri, err := opts.bulkDataURIBuilder(elem.Tag, elem.VR, value)
			if err != nil {
				return Element{}, err
			}
			out.BulkDataURI = uri
			return out, nil
		}
		out.InlineBinary = base64.StdEncoding.EncodeToString(value)
		return out, nil
	}

	values, err := elementValues(elem, opts)
	if err != nil {
		return Element{}, err
	}
	if len(values) == 0 {
		return out, nil
	}
	out.Value = values
	return out, nil
}

func elementValues(elem *godicom.DataElement, opts options) ([]json.RawMessage, error) {
	if elem.VR == godicom.VRSQ {
		seq, ok := elem.Value.(*godicom.Sequence)
		if !ok || seq == nil || seq.IsEmpty() {
			return []json.RawMessage{}, nil
		}
		values := make([]json.RawMessage, 0, seq.Len())
		for _, item := range seq.Items() {
			m, err := datasetToMap(item, opts)
			if err != nil {
				return nil, err
			}
			data, err := json.Marshal(m)
			if err != nil {
				return nil, err
			}
			values = append(values, data)
		}
		return values, nil
	}

	items := valueItems(elem.Value)
	if len(items) == 0 {
		return []json.RawMessage{}, nil
	}
	values := make([]json.RawMessage, 0, len(items))
	for _, item := range items {
		data, err := marshalValue(elem.VR, item)
		if err != nil {
			return nil, err
		}
		values = append(values, data)
	}
	return values, nil
}

func marshalValue(vr godicom.VR, value interface{}) (json.RawMessage, error) {
	if value == nil {
		return []byte("null"), nil
	}

	switch vr {
	case godicom.VRPN:
		pn, ok := value.(godicom.PersonName)
		if !ok {
			pn = godicom.ParsePersonName(fmt.Sprint(value))
		}
		return json.Marshal(personNameComponents(pn))
	case godicom.VRDA:
		if da, ok := value.(godicom.DA); ok {
			return json.Marshal(da.String())
		}
		if s, ok := value.(string); ok {
			return json.Marshal(s)
		}
	case godicom.VRTM:
		if tm, ok := value.(godicom.TM); ok {
			return json.Marshal(tm.String())
		}
		if s, ok := value.(string); ok {
			return json.Marshal(s)
		}
	case godicom.VRDT:
		if dt, ok := value.(godicom.DT); ok {
			return json.Marshal(dt.String())
		}
		if s, ok := value.(string); ok {
			return json.Marshal(s)
		}
	case godicom.VRAT:
		t, ok := value.(godicom.Tag)
		if !ok {
			if i, intOK := value.(int); intOK {
				t = godicom.Tag(i)
			} else {
				return nil, fmt.Errorf("dicomjson: AT value has type %T", value)
			}
		}
		return json.Marshal(t.JSONKey())
	}

	if isIntegerVR(vr) {
		return json.Marshal(integerJSONValue(value))
	}
	if isFloatVR(vr) {
		return json.Marshal(floatJSONValue(value))
	}
	if b, ok := value.([]byte); ok && godicom.IsStringVR(vr) {
		return json.Marshal(string(b))
	}
	return json.Marshal(value)
}

func personNameComponents(pn godicom.PersonName) map[string]string {
	components := map[string]string{}
	if pn.Alphabetic != "" {
		components["Alphabetic"] = pn.Alphabetic
	}
	if pn.Ideographic != "" {
		components["Ideographic"] = pn.Ideographic
	}
	if pn.Phonetic != "" {
		components["Phonetic"] = pn.Phonetic
	}
	return components
}

func binaryValue(elem *godicom.DataElement) []byte {
	if elem.RawValue != nil {
		return elem.RawValue
	}
	if value, ok := elem.Value.([]byte); ok {
		return value
	}
	return nil
}

func valueItems(value interface{}) []interface{} {
	switch v := value.(type) {
	case nil:
		return []interface{}{}
	case *godicom.MultiValue[interface{}]:
		return v.Values()
	case *godicom.MultiValue[int]:
		vals := v.Values()
		items := make([]interface{}, 0, len(vals))
		for _, item := range vals {
			items = append(items, item)
		}
		return items
	case *godicom.MultiValue[int64]:
		vals := v.Values()
		items := make([]interface{}, 0, len(vals))
		for _, item := range vals {
			items = append(items, item)
		}
		return items
	case *godicom.MultiValue[uint64]:
		vals := v.Values()
		items := make([]interface{}, 0, len(vals))
		for _, item := range vals {
			items = append(items, item)
		}
		return items
	case *godicom.MultiValue[float64]:
		vals := v.Values()
		items := make([]interface{}, 0, len(vals))
		for _, item := range vals {
			items = append(items, item)
		}
		return items
	case *godicom.MultiValue[godicom.Tag]:
		vals := v.Values()
		items := make([]interface{}, 0, len(vals))
		for _, item := range vals {
			items = append(items, item)
		}
		return items
	case *godicom.MultiValue[float32]:
		vals := v.Values()
		items := make([]interface{}, 0, len(vals))
		for _, item := range vals {
			items = append(items, item)
		}
		return items
	case *godicom.MultiValue[int32]:
		vals := v.Values()
		items := make([]interface{}, 0, len(vals))
		for _, item := range vals {
			items = append(items, item)
		}
		return items
	case *godicom.MultiValue[int16]:
		vals := v.Values()
		items := make([]interface{}, 0, len(vals))
		for _, item := range vals {
			items = append(items, item)
		}
		return items
	case *godicom.MultiValue[uint16]:
		vals := v.Values()
		items := make([]interface{}, 0, len(vals))
		for _, item := range vals {
			items = append(items, item)
		}
		return items
	case *godicom.MultiValue[uint32]:
		vals := v.Values()
		items := make([]interface{}, 0, len(vals))
		for _, item := range vals {
			items = append(items, item)
		}
		return items
	}
	return []interface{}{value}
}

func integerJSONValue(value interface{}) interface{} {
	switch v := value.(type) {
	case int:
		return v
	case int16:
		return int64(v)
	case int32:
		return int64(v)
	case int64:
		return v
	case uint16:
		return int64(v)
	case uint32:
		return int64(v)
	case string:
		if v == "" {
			return nil
		}
		if i, err := strconv.ParseInt(v, 10, 64); err == nil {
			return i
		}
	}
	return value
}

func floatJSONValue(value interface{}) interface{} {
	switch v := value.(type) {
	case float32:
		return float64(v)
	case float64:
		return v
	case string:
		if v == "" {
			return nil
		}
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return value
}

func coerceParsedValue(vr godicom.VR, value interface{}) (interface{}, error) {
	if value == nil {
		return emptyJSONValue(vr), nil
	}
	switch vr {
	case godicom.VRUL:
		if i, ok := value.(int64); ok {
			return uint32(i), nil
		}
	case godicom.VRSL:
		if i, ok := value.(int64); ok {
			return int32(i), nil
		}
	case godicom.VRUS:
		if i, ok := value.(int64); ok {
			return uint16(i), nil
		}
	case godicom.VRSS:
		if i, ok := value.(int64); ok {
			return int16(i), nil
		}
	case godicom.VRFL:
		if f, ok := value.(float64); ok {
			return float32(f), nil
		}
	case godicom.VRFD:
		if f, ok := value.(float64); ok {
			return f, nil
		}
	case godicom.VRIS:
		if i, ok := value.(int64); ok {
			return int(i), nil
		}
	case godicom.VRDS:
		if f, ok := value.(float64); ok {
			return strconv.FormatFloat(f, 'g', -1, 64), nil
		}
	case godicom.VRDA:
		if s, ok := value.(string); ok {
			return godicom.ParseDA(s)
		}
	case godicom.VRTM:
		if s, ok := value.(string); ok {
			return godicom.ParseTM(s)
		}
	case godicom.VRDT:
		if s, ok := value.(string); ok {
			return godicom.ParseDT(s)
		}
	}
	return value, nil
}
