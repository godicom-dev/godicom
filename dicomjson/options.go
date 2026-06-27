package dicomjson

import "github.com/godicom-dev/godicom"

type options struct {
	bulkDataThreshold   int
	bulkDataURIReader   BulkDataURIReader
	bulkDataURIBuilder  BulkDataURIBuilder
	suppressInvalidTags bool
}

// Option configures DICOM JSON Model parsing or marshaling.
type Option func(*options)

// BulkDataURIReader resolves a BulkDataURI while parsing JSON.
type BulkDataURIReader func(tag godicom.Tag, vr godicom.VR, uri string) ([]byte, error)

// BulkDataURIBuilder returns a BulkDataURI while marshaling large binary values.
type BulkDataURIBuilder func(tag godicom.Tag, vr godicom.VR, value []byte) (string, error)

func defaultOptions() options {
	return options{bulkDataThreshold: 1024}
}

// WithBulkDataThreshold sets the base64 size threshold for BulkDataURI output.
func WithBulkDataThreshold(n int) Option {
	return func(o *options) {
		o.bulkDataThreshold = n
	}
}

// WithBulkDataURIReader sets the callback used to resolve BulkDataURI values.
func WithBulkDataURIReader(fn BulkDataURIReader) Option {
	return func(o *options) {
		o.bulkDataURIReader = fn
	}
}

// WithBulkDataURIBuilder sets the callback used to produce BulkDataURI values.
func WithBulkDataURIBuilder(fn BulkDataURIBuilder) Option {
	return func(o *options) {
		o.bulkDataURIBuilder = fn
	}
}

// WithSuppressInvalidTags drops elements that fail marshaling.
func WithSuppressInvalidTags() Option {
	return func(o *options) {
		o.suppressInvalidTags = true
	}
}

func applyOptions(opts []Option) options {
	o := defaultOptions()
	for _, opt := range opts {
		if opt != nil {
			opt(&o)
		}
	}
	return o
}
