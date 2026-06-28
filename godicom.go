// Package godicom provides DICOM file reading, writing, and data manipulation.
//
// Usage:
//
//	ds, err := godicom.ReadFile("file.dcm", nil)
//	if err != nil { ... }
//
//	// 带选项：godicom.ReadFile("file.dcm", &godicom.ReadOptions{Force: true})
//	fmt.Println(ds)
//
//	ds.Set(godicom.NewElement(godicom.MustTag(0x00100010), godicom.VRPN, "Test^Patient"))
//	ds.SaveAs("output.dcm", nil)
//
// DcmRead, DcmReadFile, and DcmWrite are deprecated aliases; use ReadFile and WriteFile.
package godicom
