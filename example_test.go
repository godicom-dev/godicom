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

// ExampleStandard reads an element's entry out of the PS3.6 data dictionary. The
// second argument is the Private Creator, which a standard tag ignores.
func ExampleStandard() {
	entry, ok := godicom.Standard().Lookup(tag.PatientName, "")
	fmt.Println(ok, entry.VR, entry.VM, entry.Name, entry.Keyword, entry.Retired)
	// Output: true PN 1 Patient's Name PatientName false
}

// ExampleDictEntry_VRs shows the entries PS3.6 gives more than one VR for. Pixel
// Data is one of them, so this is not a corner case: "OB or OW" is prose, and
// comparing it against a VR from a file matches neither of the two it names.
func ExampleDictEntry_VRs() {
	entry, _ := godicom.Standard().Lookup(tag.PixelData, "")
	fmt.Println(entry.VR, "->", entry.VRs())
	// Output: OB or OW -> [OB OW]
}

// ExampleAddPrivateDictEntry registers a vendor's element, which Lookup then
// resolves like any other -- given the creator. Without one there is no entry to
// find, because the same private tag means different things to different vendors.
func ExampleAddPrivateDictEntry() {
	defer godicom.ResetExtraPrivateDictionaries()

	private := godicom.NewTag(0x0041, 0x0001)
	if err := godicom.AddPrivateDictEntry("ACME 3.2", private, godicom.VRUS, "Some Number"); err != nil {
		log.Fatal(err)
	}

	entry, ok := godicom.Standard().Lookup(private, "ACME 3.2")
	fmt.Println(ok, entry.VR, entry.Name)
	_, ok = godicom.Standard().Lookup(private, "")
	fmt.Println("without a creator:", ok)
	// Output:
	// true US Some Number
	// without a creator: false
}

// ExampleNewPrivateDictionary reads a vendor's private element as the vendor
// meant it, without registering anything process-wide. AddPrivateDictEntry above
// does the same job for a program that owns its process; this is the version a
// library can use, because the dictionary belongs to the one read that was given
// it.
//
// Implicit VR is what makes the difference visible: the file carries no VRs at
// all, so every element's VR is whatever the dictionary says, and for a private
// element that answer depends on who wrote the file.
func ExampleNewPrivateDictionary() {
	vendor := godicom.NewPrivateDictionary()
	private := godicom.NewTag(0x0041, 0x1001)
	if err := vendor.Add("ACME 3.2", private, godicom.VRUS, "Some Number"); err != nil {
		log.Fatal(err)
	}

	ds := godicom.NewDataset()
	ds.Set(godicom.NewDataElement(godicom.NewTag(0x0041, 0x0010), godicom.VRLO, "ACME 3.2"))
	ds.Set(godicom.NewDataElement(private, godicom.VRUS, 4095))
	data, err := ds.Encode(uid.ImplicitVRLittleEndian)
	if err != nil {
		log.Fatal(err)
	}

	plain, err := godicom.ReadBytes(data, &godicom.ReadOptions{Force: true})
	if err != nil {
		log.Fatal(err)
	}
	elem, _ := plain.Get(private)
	fmt.Println("read against PS3.6 alone:", elem.VR, elem.Value)

	// Compose rather than replace. Standard behind the vendor dictionary is what
	// keeps the other 5,189 entries; handing over the vendor's on its own would
	// leave every standard element as UN.
	reread, err := godicom.ReadBytes(data, &godicom.ReadOptions{
		Force:      true,
		Dictionary: godicom.NewDictionary(vendor, godicom.Standard()),
	})
	if err != nil {
		log.Fatal(err)
	}
	elem, _ = reread.Get(private)
	fmt.Println("read with the vendor's dictionary:", elem.VR, elem.Value)
	// Output:
	// read against PS3.6 alone: UN [255 15]
	// read with the vendor's dictionary: US 4095
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
