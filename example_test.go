// The examples in this file are the README's Go snippets, in a form the
// compiler and `go test` check. A README snippet that stops compiling is
// invisible until a reader tries it; one of these fails CI.
//
// They deliberately never touch the filesystem. An Example cannot take a
// *testing.T and so has no t.TempDir, and a doc example that opens a path the
// reader does not have is worse documentation than one that builds its own
// bytes. Everything here is a dataset encoded in memory and read back.

package godicom_test

import (
	"fmt"
	"log"

	"github.com/godicom-dev/godicom"
	"github.com/godicom-dev/godicom/tag"
	"github.com/godicom-dev/godicom/uid"
)

// partTen encodes ds as Part 10 bytes -- preamble, DICM, File Meta, dataset --
// the way the examples below need to read them back. SOP Class and Instance UID
// are not godicom's ceremony: PS3.10 requires File Meta to name them, and
// EnforceFileFormat fills the File Meta from the dataset.
func partTen(ds *godicom.Dataset) []byte {
	if err := ds.SetString(tag.SOPClassUID, string(uid.CTImageStorage)); err != nil {
		log.Fatal(err)
	}
	if err := ds.SetString(tag.SOPInstanceUID, "1.2.826.0.1.3680043.10.1337.1"); err != nil {
		log.Fatal(err)
	}
	fd := &godicom.FileDataset{Dataset: ds, FileMeta: godicom.NewFileMetaDataset()}
	data, err := godicom.EncodeFile(fd, &godicom.WriteOptions{EnforceFileFormat: true})
	if err != nil {
		log.Fatal(err)
	}
	return data
}

// Set a value and read it back out of the encoded bytes.
func Example() {
	ds := godicom.NewDataset()
	if err := ds.SetString(tag.PatientID, "12345678"); err != nil {
		log.Fatal(err)
	}
	if err := ds.SetString(tag.PatientName, "Doe^Jane"); err != nil {
		log.Fatal(err)
	}

	reread, err := godicom.ReadBytes(partTen(ds), nil)
	if err != nil {
		log.Fatal(err)
	}
	id, _ := reread.GetString(tag.PatientID)
	name, _ := reread.GetString(tag.PatientName)
	fmt.Println(id, name)
	// Output: 12345678 Doe^Jane
}

// ExampleReadOptions shows the diagnostic hook on the way in. A read keeps
// whatever it parsed before the file stopped making sense; the hook is how you
// find out that it did.
func ExampleReadOptions() {
	ds := godicom.NewDataset()
	if err := ds.SetString(tag.PatientName, "Doe^Jane"); err != nil {
		log.Fatal(err)
	}
	data := partTen(ds)

	// Cut the last four bytes. PatientName sorts last, so it is the element that
	// now declares more value bytes than the file holds.
	reread, err := godicom.ReadBytes(data[:len(data)-4], &godicom.ReadOptions{
		OnDiagnostic: func(d godicom.Diagnostic) error {
			fmt.Printf("%s at %s: need %d, have %d\n", d.Kind, d.Tag, d.Need, d.Have)
			return nil // keep what was parsed
		},
	})
	if err != nil {
		log.Fatal(err)
	}
	_, ok := reread.GetString(tag.PatientName)
	fmt.Println("PatientName present:", ok)
	// Output:
	// truncated_value at (0010,0010): need 8, have 4
	// PatientName present: false
}

// ExampleWriteOptions shows the same hook on the way out, refusing to produce a
// file godicom's own reader would raise a diagnostic on. "3000000000" is a
// well-formed integer string that no IS may hold, and no setter can catch that
// without knowing which VR the tag turns out to have.
//
// Set as text rather than through SetInt because the IS range is exactly the
// int32 range: on a 32-bit build no Go int is out of range, so there would be
// nothing to demonstrate.
func ExampleWriteOptions() {
	ds := godicom.NewDataset()
	if err := ds.SetString(tag.EchoNumbers, "3000000000"); err != nil {
		log.Fatal(err)
	}

	_, err := godicom.EncodeFile(&godicom.FileDataset{Dataset: ds}, &godicom.WriteOptions{
		OnDiagnostic: func(d godicom.Diagnostic) error { return d },
	})
	fmt.Println(err)
	// Output: godicom: error writing dataset: godicom: invalid_value at (0018,0086) IS: "3000000000" is outside [-2147483648, 2147483647], the range an IS allows
}

// ExampleDataset_Encode encodes a dataset without a Part 10 preamble or File
// Meta, which is what a DIMSE or multipart payload carries.
func ExampleDataset_Encode() {
	ds := godicom.NewDataset()
	if err := ds.SetString(tag.Modality, "CT"); err != nil {
		log.Fatal(err)
	}

	data, err := ds.Encode(uid.ExplicitVRLittleEndian)
	if err != nil {
		log.Fatal(err)
	}
	parsed, err := godicom.DecodeDataset(data, uid.ExplicitVRLittleEndian)
	if err != nil {
		log.Fatal(err)
	}
	modality, _ := parsed.GetString(tag.Modality)
	fmt.Printf("%d bytes, Modality %s\n", len(data), modality)
	// Output: 10 bytes, Modality CT
}
