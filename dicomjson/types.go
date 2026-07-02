// Package dicomjson converts DICOM datasets to and from JSON.
package dicomjson

import (
	"encoding/json"
)

// Element is the DICOM JSON Model representation of a data element.
type Element struct {
	VR           string            `json:"vr"`
	Value        []json.RawMessage `json:"Value,omitempty"`
	InlineBinary string            `json:"InlineBinary,omitempty"`
	BulkDataURI  string            `json:"BulkDataURI,omitempty"`
}

type rawElement struct {
	VR           string          `json:"vr"`
	Value        json.RawMessage `json:"Value"`
	InlineBinary json.RawMessage `json:"InlineBinary"`
	BulkDataURI  json.RawMessage `json:"BulkDataURI"`
}
