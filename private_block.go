package godicom

import (
	"fmt"
)

// A private block is PS3.5's answer to a question no standard element has: two
// vendors both want element 1 of group 0041, and neither will move. PS3.5 §7.8.1
// resolves it by making the high byte of the element number a *block* a vendor
// reserves at run time. The vendor writes its name into (gggg,00xx) -- the
// Private Creator -- and its elements then live at (gggg,xx01), (gggg,xx02) and
// so on, where xx is whatever block it got.
//
// So a private element's tag is not fixed by the vendor's documentation. GE's
// documented "element 1 of GEMS_ACQU_01" is (0019,1001) in one file and
// (0019,2001) in the next, depending on which block was free when each file was
// written. Application code that hardcodes (0019,1001) reads the wrong vendor's
// data the first time it meets a file where the blocks landed differently -- and
// reads it *successfully*, because there is nothing about the bytes to object to.

// PrivateBlock is the indirection that removes the hardcoding: name the vendor,
// get the block it actually occupies in this dataset, and address elements by the
// offset the vendor documents.
type PrivateBlock struct {
	// Group is the private group the block sits in, always odd.
	Group uint16
	// PrivateCreator is the vendor name in the (Group,00xx) element that reserves
	// the block.
	PrivateCreator string

	dataset *Dataset
	// blockStart is the block's base element number: the Private Creator's own
	// element number shifted into the high byte, so the low byte is always zero.
	// (0019,0010) reserves block 0x10 and gives blockStart 0x1000.
	blockStart uint16
}

// privateBlockKey identifies a block for the per-dataset cache. A struct rather
// than the [2]interface{} it used to be: the same two fields, but comparable
// without boxing and without a nil-vs-typed-nil hazard in the key.
type privateBlockKey struct {
	group   uint16
	creator string
}

// privateBlockRange is the element numbers PS3.5 §7.8.1 allows a Private Creator
// to occupy. 0x00-0x0F is reserved -- (gggg,0000) is the group length and the
// rest is unassignable -- so block 0x00 does not exist and neither does an
// element at (gggg,00xx) for xx below 0x10.
const (
	firstPrivateBlock = 0x10
	lastPrivateBlock  = 0xFF
)

// PrivateBlock returns the block that creator has reserved in group, and whether
// it has reserved one at all.
//
//	pb, ok := ds.PrivateBlock(0x0019, "GEMS_ACQU_01")
//	if !ok {
//		// this file has nothing from that vendor in that group
//	}
//	elem, ok := pb.Get(0x10)
//
// ok is false when the group holds no Private Creator with that name, which is
// the ordinary case for a file from a different manufacturer, and also when group
// is even -- an even group is a standard group and cannot contain a private block
// at all, so there is nothing to find rather than an error to report.
//
// The creator is matched exactly, as pydicom matches it. A value read from a file
// has already had its PS3.5 padding removed by the time it gets here, so the name
// to pass is the one the vendor documents; a creator that was Set by hand with a
// trailing space is a different creator.
//
// The lowest block wins if the same creator somehow reserved two, so repeated
// calls agree with each other. A well-formed file has no such thing, but the
// element map is unordered and picking whichever came out first would make the
// answer vary between runs of the same program on the same file.
//
// Use NewPrivateBlock to write private data: it reserves a block when the creator
// has none yet, which is what a dataset being built from nothing always needs.
func (d *Dataset) PrivateBlock(group uint16, creator string) (*PrivateBlock, bool) {
	if group%2 == 0 || creator == "" {
		return nil, false
	}
	key := privateBlockKey{group: group, creator: creator}
	if pb, ok := d.privateBlocks[key]; ok {
		return pb, true
	}
	element, ok := d.findPrivateCreator(group, creator)
	if !ok {
		return nil, false
	}
	return d.cachePrivateBlock(key, element), true
}

// NewPrivateBlock returns the block creator has reserved in group, reserving one
// if it has none yet. It is PrivateBlock plus the ability to write, which is what
// a dataset being built from nothing needs: with no Private Creator element there
// is no block, and without a block there is nowhere to put a private element that
// a reader could ever resolve.
//
//	pb, err := ds.NewPrivateBlock(0x0041, "ACME 3.2")
//	if err != nil {
//		return err
//	}
//	pb.Set(0x01, VRUS, 4095)
//
// That writes two elements, not one: (0041,0010) holding "ACME 3.2" if it was not
// there already, and (0041,1001) holding the value. The creator element is not
// bookkeeping to be tidied away -- it is the only thing that tells a reader whose
// element (0041,1001) is.
//
// Reserving picks the lowest free block, so a second vendor added to the same
// group lands at 0x11 rather than colliding. An existing block is returned as it
// stands and nothing is written.
//
// It returns an error rather than reserving when group is even (a standard group
// has no private blocks), when creator is empty (a block reserved under no name
// is a block nothing can resolve), or when all 240 blocks in the group are taken.
// The last cannot happen in a file anyone has written; it is reported rather than
// silently returning block 0x00, which is reserved and would produce elements no
// reader can attribute.
func (d *Dataset) NewPrivateBlock(group uint16, creator string) (*PrivateBlock, error) {
	if group%2 == 0 {
		return nil, fmt.Errorf("godicom: group %04X is not private, so it has no private blocks", group)
	}
	if creator == "" {
		return nil, fmt.Errorf("godicom: a private block needs a Private Creator, which cannot be empty")
	}
	key := privateBlockKey{group: group, creator: creator}
	if pb, ok := d.privateBlocks[key]; ok {
		return pb, nil
	}
	if element, ok := d.findPrivateCreator(group, creator); ok {
		return d.cachePrivateBlock(key, element), nil
	}

	for element := firstPrivateBlock; element <= lastPrivateBlock; element++ {
		tag := NewTag(int(group), element)
		if d.Has(tag) {
			continue
		}
		d.Set(NewDataElement(tag, VRLO, creator))
		return d.cachePrivateBlock(key, uint16(element)), nil
	}
	return nil, fmt.Errorf(
		"godicom: every private block in group %04X is reserved, so %q cannot have one",
		group, creator,
	)
}

// PrivateCreators returns the vendor names that have reserved a block in group,
// ordered by the block they occupy.
//
// This is how to approach a file from a manufacturer you have no documentation
// for: the tags themselves say nothing, but the creator names say who to ask.
//
//	for _, creator := range ds.PrivateCreators(0x0019) {
//		pb, _ := ds.PrivateBlock(0x0019, creator)
//		...
//	}
//
// An even group has no private blocks and so returns nothing, as does a group
// with no private elements in it.
func (d *Dataset) PrivateCreators(group uint16) []string {
	if group%2 == 0 {
		return nil
	}
	var creators []string
	for element := firstPrivateBlock; element <= lastPrivateBlock; element++ {
		elem, ok := d.Get(NewTag(int(group), element))
		if !ok {
			continue
		}
		if name, ok := elem.Value.(string); ok {
			creators = append(creators, name)
		}
	}
	return creators
}

// findPrivateCreator returns the element number of the Private Creator naming
// creator in group, scanning in block order so the answer does not depend on map
// iteration order.
func (d *Dataset) findPrivateCreator(group uint16, creator string) (uint16, bool) {
	for element := firstPrivateBlock; element <= lastPrivateBlock; element++ {
		elem, ok := d.Get(NewTag(int(group), element))
		if !ok {
			continue
		}
		if name, ok := elem.Value.(string); ok && name == creator {
			return uint16(element), true
		}
	}
	return 0, false
}

// cachePrivateBlock builds the block for a Private Creator at element and
// remembers it, so two lookups of the same block return the same value rather
// than rescanning the group.
func (d *Dataset) cachePrivateBlock(key privateBlockKey, element uint16) *PrivateBlock {
	pb := &PrivateBlock{
		Group:          key.group,
		PrivateCreator: key.creator,
		dataset:        d,
		blockStart:     element << 8,
	}
	d.privateBlocks[key] = pb
	return pb
}

// invalidatePrivateBlocks drops the cache. Called when an element that reserves a
// block goes away: a cached block outliving its Private Creator would keep
// answering with a blockStart nothing in the dataset attributes any more, and
// writing through it would produce private elements no reader can resolve.
func (d *Dataset) invalidatePrivateBlocks() {
	d.privateBlocks = map[privateBlockKey]*PrivateBlock{}
}

// GetTag returns the tag offset addresses within the block.
//
// offset is uint8 because that is exactly the range PS3.5 §7.8.1 allows -- the
// block occupies the high byte of the element number and the offset is the low
// byte. pydicom takes an int and raises ValueError above 0xFF; in Go the type
// does that work, and the check has nowhere left to fail.
//
//	pb.GetTag(0x01) // (0019,1001) for a block at 0x10
func (pb *PrivateBlock) GetTag(offset uint8) Tag {
	return NewTag(int(pb.Group), int(pb.blockStart)+int(offset))
}

// Get returns the element at offset within the block, and whether it is present.
func (pb *PrivateBlock) Get(offset uint8) (*DataElement, bool) {
	return pb.dataset.Get(pb.GetTag(offset))
}

// Set stores value at offset within the block, replacing whatever was there.
//
// The VR is given rather than looked up: a private element's VR depends on the
// vendor, and the standard dictionary has no entry to consult. A vendor's
// dictionary can supply one -- see NewPrivateDictionary -- but that resolves
// elements on the way in, not on the way out.
func (pb *PrivateBlock) Set(offset uint8, vr VR, value any) {
	pb.dataset.Set(NewDataElement(pb.GetTag(offset), vr, value))
}

// Delete removes the element at offset within the block, and does nothing if it
// was not there.
//
// It does not release the block. The Private Creator element stays, so the block
// is still reserved and still resolves; deleting the last element of a block
// leaves an empty reservation rather than freeing it for another vendor.
func (pb *PrivateBlock) Delete(offset uint8) {
	pb.dataset.Delete(pb.GetTag(offset))
}
