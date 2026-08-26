package godicom

import (
	"strings"
	"testing"
)

// creatorSet builds the dataset pydicom's private-block tests build: one standard
// element, two creators in non-contiguous blocks, and elements in each.
func creatorSet(t *testing.T) *Dataset {
	t.Helper()
	ds := NewDataset()
	ds.Set(NewDataElement(MustTag(0x00080005), VRCS, "ISO_IR 100"))
	ds.Set(NewDataElement(MustTag(0x00090010), VRLO, "Creator 1.0"))
	ds.Set(NewDataElement(MustTag(0x00091001), VRSH, "Version1"))
	ds.Set(NewDataElement(MustTag(0x00090020), VRLO, "Creator 2.0"))
	ds.Set(NewDataElement(MustTag(0x00092001), VRSH, "Version2"))
	ds.Set(NewDataElement(MustTag(0x00092002), VRUS, uint16(2)))
	return ds
}

func TestPrivateBlockLookup(t *testing.T) {
	// pydicom.tests.test_dataset.TestDataset.test_private_block
	ds := creatorSet(t)

	// pydicom raises for these three; the ok idiom reports all three the same way,
	// because to a caller holding a file from another vendor they are one case:
	// there is no such block here.
	for _, tc := range []struct {
		name    string
		group   uint16
		creator string
	}{
		{"even group", 0x0008, "Creator 1.0"},
		{"empty creator", 0x0009, ""},
		{"unknown creator", 0x0009, "Creator 3.0"},
	} {
		pb, ok := ds.PrivateBlock(tc.group, tc.creator)
		if ok {
			t.Errorf("%s: PrivateBlock ok = true, want false", tc.name)
		}
		if pb != nil {
			t.Errorf("%s: PrivateBlock = %v, want nil", tc.name, pb)
		}
	}

	first, ok := ds.PrivateBlock(0x0009, "Creator 1.0")
	if !ok {
		t.Fatal("Creator 1.0 block not found")
	}
	if got, want := first.GetTag(0x01), MustTag(0x00091001); got != want {
		t.Errorf("GetTag(0x01) = %s, want %s", got, want)
	}
	elem, ok := first.Get(0x01)
	if !ok {
		t.Fatal("(0009,1001) not found through the block")
	}
	if elem.Value != "Version1" {
		t.Errorf("offset 0x01 = %v, want Version1", elem.Value)
	}
	if _, ok := first.Get(0x02); ok {
		t.Error("offset 0x02 found, but Creator 1.0 has no element there")
	}

	// The blocks are non-contiguous on purpose: Creator 2.0 is at 0x20, not 0x11,
	// which is what a file gets when a vendor's writer picks a block by its own
	// rules rather than counting up from 0x10.
	second, ok := ds.PrivateBlock(0x0009, "Creator 2.0")
	if !ok {
		t.Fatal("Creator 2.0 block not found")
	}
	if got, want := second.GetTag(0x01), MustTag(0x00092001); got != want {
		t.Errorf("GetTag(0x01) = %s, want %s", got, want)
	}
	elem, ok = second.Get(0x01)
	if !ok {
		t.Fatal("(0009,2001) not found through the block")
	}
	if elem.Value != "Version2" {
		t.Errorf("offset 0x01 = %v, want Version2", elem.Value)
	}
	elem, ok = second.Get(0x02)
	if !ok {
		t.Fatal("(0009,2002) not found through the block")
	}
	if elem.Value != uint16(2) {
		t.Errorf("offset 0x02 = %v, want 2", elem.Value)
	}
}

func TestPrivateBlockFields(t *testing.T) {
	ds := creatorSet(t)
	pb, ok := ds.PrivateBlock(0x0009, "Creator 2.0")
	if !ok {
		t.Fatal("Creator 2.0 block not found")
	}
	if pb.Group != 0x0009 {
		t.Errorf("Group = %04X, want 0009", pb.Group)
	}
	if pb.PrivateCreator != "Creator 2.0" {
		t.Errorf("PrivateCreator = %q, want Creator 2.0", pb.PrivateCreator)
	}
}

func TestPrivateBlockCached(t *testing.T) {
	ds := creatorSet(t)
	first, _ := ds.PrivateBlock(0x0009, "Creator 1.0")
	again, _ := ds.PrivateBlock(0x0009, "Creator 1.0")
	if first != again {
		t.Error("two lookups of the same block returned different values")
	}
}

// A creator that appears twice is malformed, but the answer still has to be the
// same one every time: the element map is unordered, and choosing whichever came
// out of it first would make a program's behaviour vary between runs on one file.
func TestPrivateBlockLowestBlockWins(t *testing.T) {
	for i := 0; i < 50; i++ {
		ds := NewDataset()
		ds.Set(NewDataElement(MustTag(0x00090020), VRLO, "DUPLICATED"))
		ds.Set(NewDataElement(MustTag(0x00090010), VRLO, "DUPLICATED"))
		ds.Set(NewDataElement(MustTag(0x000900F0), VRLO, "DUPLICATED"))

		pb, ok := ds.PrivateBlock(0x0009, "DUPLICATED")
		if !ok {
			t.Fatal("block not found")
		}
		if got, want := pb.GetTag(0x01), MustTag(0x00091001); got != want {
			t.Fatalf("GetTag(0x01) = %s, want %s (lowest block)", got, want)
		}
	}
}

// PS3.5 §7.8.1 reserves elements below 0x0010, so nothing there names a block.
// An element that looks like a creator but sits at 0x0002 reserves nothing.
func TestPrivateBlockReservedElementsAreNotBlocks(t *testing.T) {
	ds := NewDataset()
	ds.Set(NewDataElement(MustTag(0x00090002), VRLO, "NOT_A_BLOCK"))

	if _, ok := ds.PrivateBlock(0x0009, "NOT_A_BLOCK"); ok {
		t.Error("an element below 0x0010 was treated as a Private Creator")
	}
	if got := ds.PrivateCreators(0x0009); len(got) != 0 {
		t.Errorf("PrivateCreators = %v, want none", got)
	}
}

func TestNewPrivateBlockReserves(t *testing.T) {
	// pydicom.tests.test_dataset.TestDataset.test_add_new_private_tag
	ds := NewDataset()
	ds.Set(NewDataElement(MustTag(0x00080005), VRCS, "ISO_IR 100"))
	ds.Set(NewDataElement(MustTag(0x00090010), VRLO, "Creator 1.0"))
	ds.Set(NewDataElement(MustTag(0x00090011), VRLO, "Creator 2.0"))

	if _, err := ds.NewPrivateBlock(0x0008, "Creator 1.0"); err == nil {
		t.Error("NewPrivateBlock on an even group returned no error")
	}

	// An existing creator is found, not re-reserved.
	pb, err := ds.NewPrivateBlock(0x0009, "Creator 2.0")
	if err != nil {
		t.Fatal(err)
	}
	pb.Set(0x01, VRSH, "Version2")
	elem, ok := ds.Get(MustTag(0x00091101))
	if !ok {
		t.Fatal("(0009,1101) not written")
	}
	if elem.Value != "Version2" {
		t.Errorf("(0009,1101) = %v, want Version2", elem.Value)
	}

	// A new creator takes the lowest free block, which is 0x12 with 0x10 and 0x11
	// already taken.
	pb, err = ds.NewPrivateBlock(0x0009, "Creator 3.0")
	if err != nil {
		t.Fatal(err)
	}
	creator, ok := ds.Get(MustTag(0x00090012))
	if !ok {
		t.Fatal("(0009,0012) creator element not written")
	}
	if creator.Value != "Creator 3.0" {
		t.Errorf("(0009,0012) = %v, want Creator 3.0", creator.Value)
	}
	if creator.VR != VRLO {
		t.Errorf("(0009,0012) VR = %s, want LO", creator.VR)
	}
	pb.Set(0x01, VRSH, "Version3")
	elem, ok = ds.Get(MustTag(0x00091201))
	if !ok {
		t.Fatal("(0009,1201) not written")
	}
	if elem.Value != "Version3" {
		t.Errorf("(0009,1201) = %v, want Version3", elem.Value)
	}
}

// The case a dataset built from nothing always hits: no creator element yet, so
// there is no block, and PrivateBlock alone can only report that.
func TestNewPrivateBlockFromEmptyDataset(t *testing.T) {
	ds := NewDataset()
	if _, ok := ds.PrivateBlock(0x0041, "ACME 3.2"); ok {
		t.Fatal("an empty dataset reported a private block")
	}

	pb, err := ds.NewPrivateBlock(0x0041, "ACME 3.2")
	if err != nil {
		t.Fatal(err)
	}
	if ds.Len() != 1 {
		t.Fatalf("Len = %d after reserving, want 1 (the creator element)", ds.Len())
	}
	creator, ok := ds.Get(MustTag(0x00410010))
	if !ok {
		t.Fatal("(0041,0010) creator element not written")
	}
	if creator.VR != VRLO || creator.Value != "ACME 3.2" {
		t.Fatalf("(0041,0010) = %s %v, want LO ACME 3.2", creator.VR, creator.Value)
	}
	if got, want := pb.GetTag(0x01), MustTag(0x00411001); got != want {
		t.Errorf("GetTag(0x01) = %s, want %s", got, want)
	}

	// A second vendor in the same group lands at the next free block rather than
	// on top of the first.
	other, err := ds.NewPrivateBlock(0x0041, "OTHER 1.0")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := other.GetTag(0x01), MustTag(0x00411101); got != want {
		t.Errorf("second creator GetTag(0x01) = %s, want %s", got, want)
	}
}

func TestNewPrivateBlockExistingIsUnchanged(t *testing.T) {
	ds := NewDataset()
	pb, err := ds.NewPrivateBlock(0x0041, "ACME 3.2")
	if err != nil {
		t.Fatal(err)
	}
	pb.Set(0x01, VRUS, uint16(4095))
	before := ds.Len()

	again, err := ds.NewPrivateBlock(0x0041, "ACME 3.2")
	if err != nil {
		t.Fatal(err)
	}
	if again != pb {
		t.Error("NewPrivateBlock reserved a second block for a creator that had one")
	}
	if ds.Len() != before {
		t.Errorf("Len = %d, want %d: nothing should have been written", ds.Len(), before)
	}
}

func TestNewPrivateBlockErrors(t *testing.T) {
	ds := NewDataset()

	if _, err := ds.NewPrivateBlock(0x0008, "ACME 3.2"); err == nil {
		t.Error("even group returned no error")
	} else if !strings.Contains(err.Error(), "not private") {
		t.Errorf("even group error = %v", err)
	}
	if _, err := ds.NewPrivateBlock(0x0009, ""); err == nil {
		t.Error("empty creator returned no error")
	} else if !strings.Contains(err.Error(), "Private Creator") {
		t.Errorf("empty creator error = %v", err)
	}
	if ds.Len() != 0 {
		t.Fatalf("Len = %d, want 0: a rejected reservation must write nothing", ds.Len())
	}

	// All 240 blocks taken. Not a file anyone has written, but reporting it is the
	// alternative to returning block 0x00, which is reserved and would produce
	// elements no reader can attribute to a vendor.
	full := NewDataset()
	for element := firstPrivateBlock; element <= lastPrivateBlock; element++ {
		if _, err := full.NewPrivateBlock(0x0009, "CREATOR "+string(rune('A'+element%26))+string(rune('A'+element/26))); err != nil {
			t.Fatalf("reserving block %02X: %v", element, err)
		}
	}
	if full.Len() != lastPrivateBlock-firstPrivateBlock+1 {
		t.Fatalf("Len = %d, want %d", full.Len(), lastPrivateBlock-firstPrivateBlock+1)
	}
	if _, err := full.NewPrivateBlock(0x0009, "ONE TOO MANY"); err == nil {
		t.Error("a full group reserved another block")
	} else if !strings.Contains(err.Error(), "reserved") {
		t.Errorf("full group error = %v", err)
	}
}

func TestPrivateCreators(t *testing.T) {
	// pydicom.tests.test_dataset.TestDataset.test_private_creators
	ds := NewDataset()
	ds.Set(NewDataElement(MustTag(0x00080005), VRCS, "ISO_IR 100"))
	ds.Set(NewDataElement(MustTag(0x00090010), VRLO, "Creator 1.0"))
	ds.Set(NewDataElement(MustTag(0x00090011), VRLO, "Creator 2.0"))

	// pydicom raises on an even group; there is nothing to enumerate either way.
	if got := ds.PrivateCreators(0x0008); got != nil {
		t.Errorf("PrivateCreators(0x0008) = %v, want nil", got)
	}
	if got := ds.PrivateCreators(0x0011); got != nil {
		t.Errorf("PrivateCreators(0x0011) = %v, want nil", got)
	}
	assertCreators(t, ds.PrivateCreators(0x0009), "Creator 1.0", "Creator 2.0")
}

func TestPrivateCreatorsNonContiguous(t *testing.T) {
	// pydicom.tests.test_dataset.TestDataset.test_non_contiguous_private_creators
	ds := NewDataset()
	ds.Set(NewDataElement(MustTag(0x00080005), VRCS, "ISO_IR 100"))
	ds.Set(NewDataElement(MustTag(0x00090010), VRLO, "Creator 1.0"))
	ds.Set(NewDataElement(MustTag(0x00090020), VRLO, "Creator 2.0"))
	ds.Set(NewDataElement(MustTag(0x000900FF), VRLO, "Creator 3.0"))

	assertCreators(t, ds.PrivateCreators(0x0009), "Creator 1.0", "Creator 2.0", "Creator 3.0")
}

func assertCreators(t *testing.T, got []string, want ...string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("PrivateCreators = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("PrivateCreators = %v, want %v", got, want)
		}
	}
}

func TestPrivateBlockDelete(t *testing.T) {
	// pydicom.tests.test_dataset.TestDataset.test_delete_private_tag
	ds := NewDataset()
	ds.Set(NewDataElement(MustTag(0x00080005), VRCS, "ISO_IR 100"))
	ds.Set(NewDataElement(MustTag(0x00090010), VRLO, "Creator 1.0"))
	ds.Set(NewDataElement(MustTag(0x00090011), VRLO, "Creator 2.0"))
	ds.Set(NewDataElement(MustTag(0x00091101), VRSH, "Version2"))

	pb, ok := ds.PrivateBlock(0x0009, "Creator 2.0")
	if !ok {
		t.Fatal("block not found")
	}
	if _, ok := pb.Get(0x01); !ok {
		t.Fatal("offset 0x01 missing before the delete")
	}

	pb.Delete(0x01)
	if _, ok := pb.Get(0x01); ok {
		t.Error("offset 0x01 still present after Delete")
	}
	pb.Delete(0x01) // deleting what is not there is a no-op, as Dataset.Delete is

	// The block itself survives: the creator element still reserves it, so an
	// emptied block is still this vendor's block and not free for another.
	if _, ok := ds.PrivateBlock(0x0009, "Creator 2.0"); !ok {
		t.Error("the block stopped resolving after its only element was deleted")
	}
	if !ds.Has(MustTag(0x00090011)) {
		t.Error("Delete removed the Private Creator element")
	}
}

// A cached block outliving the element that reserved it would keep answering with
// a block the dataset no longer attributes to anyone. pydicom's regression #1097
// is the same defect.
func TestPrivateBlockCacheAfterCreatorDeleted(t *testing.T) {
	// pydicom.tests.test_dataset.TestDataset.test_create_private_tag_after_removing_private_creator
	ds := NewDataset()
	pb, err := ds.NewPrivateBlock(0x000B, "dog^1")
	if err != nil {
		t.Fatal(err)
	}
	pb.Set(0x01, VRSH, "Border Collie")

	ds.Delete(MustTag(0x000B0010))
	if _, ok := ds.PrivateBlock(0x000B, "dog^1"); ok {
		t.Fatal("the block still resolved after its Private Creator was deleted")
	}

	pb, err = ds.NewPrivateBlock(0x000B, "dog^1")
	if err != nil {
		t.Fatal(err)
	}
	pb.Set(0x02, VRSH, "Poodle")
	// Back at 0x10, the lowest free block: creator plus the orphaned 0x01 element
	// plus the new 0x02.
	if ds.Len() != 3 {
		t.Fatalf("Len = %d, want 3", ds.Len())
	}
	creator, ok := ds.Get(MustTag(0x000B0010))
	if !ok || creator.Value != "dog^1" {
		t.Fatalf("(000B,0010) = %v, want dog^1", creator)
	}
	if got, want := pb.GetTag(0x02), MustTag(0x000B1002); got != want {
		t.Fatalf("GetTag(0x02) = %s, want %s", got, want)
	}
}

func TestPrivateBlockCacheAfterRemovePrivateTags(t *testing.T) {
	// pydicom.tests.test_dataset.TestDataset.test_create_private_tag_after_removing_all
	ds := NewDataset()
	first, err := ds.NewPrivateBlock(0x000B, "dog^1")
	if err != nil {
		t.Fatal(err)
	}
	first.Set(0x01, VRSH, "Border Collie")
	second, err := ds.NewPrivateBlock(0x000B, "dog^2")
	if err != nil {
		t.Fatal(err)
	}
	second.Set(0x01, VRSH, "Poodle")
	if ds.Len() != 4 {
		t.Fatalf("Len = %d, want 4", ds.Len())
	}

	ds.RemovePrivateTags()
	if ds.Len() != 0 {
		t.Fatalf("Len = %d after RemovePrivateTags, want 0", ds.Len())
	}

	// dog^2 held 0x11 before; with the group empty it gets 0x10.
	second, err = ds.NewPrivateBlock(0x000B, "dog^2")
	if err != nil {
		t.Fatal(err)
	}
	second.Set(0x01, VRSH, "Poodle")
	if ds.Len() != 2 {
		t.Fatalf("Len = %d, want 2", ds.Len())
	}
	creator, ok := ds.Get(MustTag(0x000B0010))
	if !ok || creator.Value != "dog^2" {
		t.Fatalf("(000B,0010) = %v, want dog^2", creator)
	}
}

func TestPrivateBlockCacheAfterClear(t *testing.T) {
	ds := NewDataset()
	if _, err := ds.NewPrivateBlock(0x000B, "dog^1"); err != nil {
		t.Fatal(err)
	}
	ds.Clear()
	if _, ok := ds.PrivateBlock(0x000B, "dog^1"); ok {
		t.Error("a block resolved out of a cleared dataset")
	}
}

// Overwriting a creator element hands its block to a different vendor. pydicom
// leaves the old name cached and pointing into the new vendor's elements.
func TestPrivateBlockCacheAfterCreatorOverwritten(t *testing.T) {
	ds := NewDataset()
	if _, err := ds.NewPrivateBlock(0x000B, "OLD 1.0"); err != nil {
		t.Fatal(err)
	}
	if _, ok := ds.PrivateBlock(0x000B, "OLD 1.0"); !ok {
		t.Fatal("the block was not there to begin with")
	}

	ds.Set(NewDataElement(MustTag(0x000B0010), VRLO, "NEW 1.0"))

	if _, ok := ds.PrivateBlock(0x000B, "OLD 1.0"); ok {
		t.Error("OLD 1.0 still resolved after its creator element was overwritten")
	}
	pb, ok := ds.PrivateBlock(0x000B, "NEW 1.0")
	if !ok {
		t.Fatal("NEW 1.0 did not resolve")
	}
	if got, want := pb.GetTag(0x01), MustTag(0x000B1001); got != want {
		t.Errorf("GetTag(0x01) = %s, want %s", got, want)
	}
}

// The point of the indirection: a block written by name resolves by name after a
// round trip, whatever block number it landed in.
// The cache holds a *Dataset, so a clone that inherited it would write the
// clone's private elements into the original. Clone starts from a fresh dataset
// and rebuilds, which is what pydicom's __deepcopy__ rebuilds each block for.
func TestPrivateBlockCloneIsIndependent(t *testing.T) {
	ds := NewDataset()
	pb, err := ds.NewPrivateBlock(0x000B, "ACME 3.2")
	if err != nil {
		t.Fatal(err)
	}
	pb.Set(0x01, VRSH, "original")

	clone := ds.Clone()
	clonePB, ok := clone.PrivateBlock(0x000B, "ACME 3.2")
	if !ok {
		t.Fatal("the clone has no such block")
	}
	clonePB.Set(0x02, VRSH, "added to the clone")

	if ds.Has(MustTag(0x000B1002)) {
		t.Error("writing through the clone's block reached the original dataset")
	}
	if !clone.Has(MustTag(0x000B1002)) {
		t.Error("the clone did not get its own element")
	}
}

func TestPrivateBlockRoundTrip(t *testing.T) {
	ds := NewDataset()
	ds.Set(NewDataElement(MustTag(0x000B0010), VRLO, "OTHER 1.0"))
	pb, err := ds.NewPrivateBlock(0x000B, "ACME 3.2")
	if err != nil {
		t.Fatal(err)
	}
	pb.Set(0x01, VRSH, "Version1")
	written := pb.GetTag(0x01)
	if want := MustTag(0x000B1101); written != want {
		t.Fatalf("wrote %s, want %s (0x10 was taken)", written, want)
	}

	encoded, err := ds.Encode(ExplicitVRLittleEndian)
	if err != nil {
		t.Fatal(err)
	}
	got, err := DecodeDataset(encoded, ExplicitVRLittleEndian)
	if err != nil {
		t.Fatal(err)
	}

	reread, ok := got.PrivateBlock(0x000B, "ACME 3.2")
	if !ok {
		t.Fatal("the block did not resolve after a round trip")
	}
	if reread.GetTag(0x01) != written {
		t.Fatalf("block moved: %s, want %s", reread.GetTag(0x01), written)
	}
	elem, ok := reread.Get(0x01)
	if !ok {
		t.Fatal("offset 0x01 missing after a round trip")
	}
	if elem.Value != "Version1" {
		t.Errorf("offset 0x01 = %v, want Version1", elem.Value)
	}
	assertCreators(t, got.PrivateCreators(0x000B), "OTHER 1.0", "ACME 3.2")
}

func TestPrivateBlockFromFile(t *testing.T) {
	// pydicom.tests.test_dataset.TestDataset.test_private_creator_from_raw_ds and
	// test_add_known_private_tag2, which read the same file.
	ds, err := ReadFile(testFilePath("CT_small.dcm"), nil)
	if err != nil {
		t.Fatal(err)
	}

	assertCreators(t, ds.PrivateCreators(0x0011), "GEMS_PATI_01")
	pb, ok := ds.PrivateBlock(0x0011, "GEMS_PATI_01")
	if !ok {
		t.Fatal("GEMS_PATI_01 block not found")
	}
	// Patient Status, which GE documents as offset 0x10 of that block.
	if got, want := pb.GetTag(0x10), MustTag(0x00111010); got != want {
		t.Fatalf("GetTag(0x10) = %s, want %s", got, want)
	}
	if _, ok := pb.Get(0x10); !ok {
		t.Error("offset 0x10 missing")
	}

	// A group the file has nothing in: reserving there starts from 0x10.
	if got := ds.PrivateCreators(0x0013); got != nil {
		t.Fatalf("PrivateCreators(0x0013) = %v, want nil", got)
	}
	fresh, err := ds.NewPrivateBlock(0x0013, "GEMS_PATI_01")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := fresh.GetTag(0x01), MustTag(0x00131001); got != want {
		t.Fatalf("GetTag(0x01) = %s, want %s", got, want)
	}
	assertCreators(t, ds.PrivateCreators(0x0013), "GEMS_PATI_01")
}
