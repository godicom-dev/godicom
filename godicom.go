// Package godicom provides DICOM file reading, writing, and data manipulation.
//
// Usage:
//
//	ds, err := godicom.DcmReadFile("file.dcm")
//	if err != nil { ... }
//	fmt.Println(ds)
//
//	ds.Set(godicom.NewDataElement(godicom.MustTag(0x00100010), godicom.VRPN, "Test^Patient"))
//	ds.SaveAs("output.dcm", nil)
package godicom
