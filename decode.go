package godicom

import (
	"context"
	"fmt"

	"github.com/godicom-dev/godicom/uid"
)

// DecodeDataset decodes a DICOM dataset (no preamble / File Meta) using
// transferSyntaxUID. Suitable for DIMSE Identifiers and C-STORE datasets.
func DecodeDataset(data []byte, transferSyntaxUID string) (*Dataset, error) {
	return DecodeDatasetContext(context.Background(), data, transferSyntaxUID)
}

// DecodeDatasetContext is like DecodeDataset but uses ctx for logging.
func DecodeDatasetContext(ctx context.Context, data []byte, transferSyntaxUID string) (*Dataset, error) {
	ts := uid.UID(transferSyntaxUID)
	info, known := uid.Known[ts]
	if !known || !info.IsTransferSyntax {
		return nil, fmt.Errorf(
			"godicom: Transfer Syntax UID %q is not a known transfer syntax; use DecodeDatasetEncoding",
			transferSyntaxUID,
		)
	}
	logDebug(loggerContext(ctx, nil, ComponentReader), "decode dataset",
		AttrTransferSyntax, transferSyntaxUID,
		AttrLen, len(data),
	)
	payload := data
	if ts.IsDeflated() {
		inflated, err := inflateRaw(data)
		if err != nil {
			return nil, fmt.Errorf("godicom: error inflating dataset: %w", err)
		}
		payload = inflated
	}
	return DecodeDatasetEncodingContext(ctx, payload, info.IsImplicitVR, info.IsLittleEndian)
}

// DecodeDatasetEncoding decodes a DICOM dataset with explicit VR/endian flags.
func DecodeDatasetEncoding(data []byte, isImplicitVR, isLittleEndian bool) (*Dataset, error) {
	return DecodeDatasetEncodingContext(context.Background(), data, isImplicitVR, isLittleEndian)
}

// DecodeDatasetEncodingContext is like DecodeDatasetEncoding but uses ctx for logging.
func DecodeDatasetEncodingContext(ctx context.Context, data []byte, isImplicitVR, isLittleEndian bool) (*Dataset, error) {
	if isImplicitVR && !isLittleEndian {
		return nil, fmt.Errorf("godicom: implicit VR and big endian is not a valid encoding combination")
	}
	ctx = loggerContext(ctx, nil, ComponentReader)
	if len(data) == 0 {
		ds := NewDataset()
		ds.SetOriginalEncoding(isImplicitVR, isLittleEndian, nil)
		return ds, nil
	}
	ds := NewDataset()
	rc := &readContext{data: data, ctx: ctx}
	_, err := readDatasetElements(data, 0, int64(len(data)), ds, isImplicitVR, isLittleEndian, nil, nil, rc)
	if err != nil {
		return nil, fmt.Errorf("godicom: error decoding dataset: %w", err)
	}
	ds.originalEnc = EncodingInfo{IsImplicitVR: isImplicitVR, IsLittleEndian: isLittleEndian}
	propagateEncoding(ds, ds.originalEnc)
	captureOriginalCharsets(ds)
	return ds, nil
}
