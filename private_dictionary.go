package godicom

import (
	"fmt"
	"sync"
)

// extraPrivateDictionary holds what AddPrivateDictEntry has registered. It is
// process-global, which is the whole reason NewPrivateDictionary exists: a
// caller that wants a private dictionary of its own builds one and hands it to
// ReadOptions.Dictionary instead of mutating this.
var extraPrivateDictionary = NewPrivateDictionary()

func privateTagKeys(tag Tag) []string {
	group := tag.Group()
	elem := tag.Element()
	groupStr := fmt.Sprintf("%04X", group)
	elemStr := fmt.Sprintf("%04X", elem)
	return []string{
		groupStr + elemStr,
		fmt.Sprintf("%sxx%02X", groupStr, elem&0xFF),
		fmt.Sprintf("%sxxxx%02X", groupStr[:2], elem&0xFF),
	}
}

// lookupPrivateEntryIn resolves tag in one creator's block, trying the three
// spellings a private entry may be keyed under: the exact tag, the tag with its
// block byte wild (0041xx01), and the tag with its group's low byte wild too.
// A vendor block is relocatable -- the same element appears at (0041,1001) in
// one file and (0041,2001) in the next -- so the wild forms are the normal case
// and the exact one is the exception.
func lookupPrivateEntryIn(
	dict map[string]map[string]privateDictEntry,
	tag Tag,
	creator string,
) (privateDictEntry, bool) {
	inner, ok := dict[creator]
	if !ok {
		return privateDictEntry{}, false
	}
	for _, key := range privateTagKeys(tag) {
		if entry, ok := inner[key]; ok {
			return entry, true
		}
	}
	return privateDictEntry{}, false
}

func lookupPrivateDictEntry(tag Tag, creator string) (privateDictEntry, bool) {
	if entry, ok := lookupPrivateEntryIn(privateDictionaries, tag, creator); ok {
		return entry, true
	}
	return extraPrivateDictionary.lookupEntry(tag, creator)
}

// PrivateDictionary is a set of private data dictionary entries, keyed by the
// Private Creator that gives them meaning. It satisfies Dictionary, so a caller
// can build one, compose it with Standard through NewDictionary, and hand the
// result to ReadOptions.Dictionary -- a private dictionary that belongs to one
// read rather than to the process.
//
// It answers only for the entries Add put in it. A standard tag has none, so a
// PrivateDictionary used on its own resolves nothing outside its own vendor
// blocks; NewDictionary is what pairs it with Standard.
//
// The zero value is not usable. Use NewPrivateDictionary.
//
// Safe for concurrent use.
type PrivateDictionary struct {
	mu      sync.RWMutex
	entries map[string]map[string]privateDictEntry
}

// NewPrivateDictionary returns an empty private dictionary ready for Add.
func NewPrivateDictionary() *PrivateDictionary {
	return &PrivateDictionary{entries: map[string]map[string]privateDictEntry{}}
}

// Add registers a vendor's element under creator. vm defaults to "1".
//
// tag must be private -- an odd group -- because a standard tag's meaning does
// not depend on who wrote the file, and registering one here would create an
// entry nothing can ever reach.
//
// The entry is stored against the tag's block byte rather than its full element
// number, so it resolves wherever the vendor's block lands: adding (0041,0001)
// answers for (0041,1001) and (0041,2001) alike. That is what makes a private
// dictionary usable at all -- PS3.5 lets a block move between files.
//
// Adding the same creator and block byte twice replaces the earlier entry.
func (d *PrivateDictionary) Add(creator string, tag Tag, vr VR, name string, vm ...string) error {
	if !tag.IsPrivate() {
		return fmt.Errorf(
			"godicom: non-private tag %s cannot be added to a private dictionary",
			tag,
		)
	}
	multiplicity := "1"
	if len(vm) > 0 {
		multiplicity = vm[0]
	}

	key := fmt.Sprintf("%04Xxx%02X", tag.Group(), tag.Element()&0xFF)
	entry := privateDictEntry{VR: string(vr), VM: multiplicity, Name: name}

	d.mu.Lock()
	defer d.mu.Unlock()
	if _, ok := d.entries[creator]; !ok {
		d.entries[creator] = map[string]privateDictEntry{}
	}
	d.entries[creator][key] = entry
	return nil
}

// Lookup implements Dictionary.
//
// Unlike Standard it does not treat an empty creator as unanswerable: a lookup
// with creator "" finds whatever was added under "". Nothing sensible can be
// registered there, but AddPrivateDictEntry has always accepted it and
// PrivateDictionaryVR has always found it back.
//
// Keyword is left empty. PS3.6 assigns keywords to standard elements, and a
// private element has none to assign.
func (d *PrivateDictionary) Lookup(tag Tag, creator string) (DictEntry, bool) {
	entry, ok := d.lookupEntry(tag, creator)
	if !ok {
		return DictEntry{}, false
	}
	return DictEntry{
		VR:      entry.VR,
		VM:      entry.VM,
		Name:    entry.Name,
		Retired: entry.Retired,
	}, true
}

func (d *PrivateDictionary) lookupEntry(tag Tag, creator string) (privateDictEntry, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return lookupPrivateEntryIn(d.entries, tag, creator)
}

// reset drops every entry. Unexported because the only caller that needs it is
// ResetExtraPrivateDictionaries, cleaning up after the process-global instance;
// a dictionary you own is discarded by dropping the pointer.
func (d *PrivateDictionary) reset() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.entries = map[string]map[string]privateDictEntry{}
}

func privateDictLookup(tag Tag, creator string) (string, bool) {
	entry, ok := lookupPrivateDictEntry(tag, creator)
	if !ok {
		return "", false
	}
	return entry.Name, true
}

func privateDictionaryVR(tag Tag, creator string) (VR, bool) {
	entry, ok := lookupPrivateDictEntry(tag, creator)
	if !ok {
		return "", false
	}
	return VR(entry.VR), true
}

// PrivateDictionaryVR returns the VR for a private element.
func PrivateDictionaryVR(tag Tag, creator string) (VR, error) {
	vr, ok := privateDictionaryVR(tag, creator)
	if !ok {
		return "", fmt.Errorf("godicom: private tag %s not found for creator %q", tag, creator)
	}
	return vr, nil
}

// PrivateDictionaryVM returns the VM for a private element.
func PrivateDictionaryVM(tag Tag, creator string) (string, error) {
	entry, ok := lookupPrivateDictEntry(tag, creator)
	if !ok {
		return "", fmt.Errorf("godicom: private tag %s not found for creator %q", tag, creator)
	}
	return entry.VM, nil
}

// PrivateDictionaryDescription returns the name for a private element.
func PrivateDictionaryDescription(tag Tag, creator string) (string, error) {
	entry, ok := lookupPrivateDictEntry(tag, creator)
	if !ok {
		return "", fmt.Errorf("godicom: private tag %s not found for creator %q", tag, creator)
	}
	return entry.Name, nil
}

// AddPrivateDictEntry adds or updates a runtime private dictionary entry, which
// Standard then resolves like any other. vm defaults to "1".
//
// It mutates process-global state. Every caller in the process sees the entry,
// including libraries that never asked for it, and the only way to remove it is
// ResetExtraPrivateDictionaries, which clears everything anyone has registered.
// That is fine for a program that owns its process and wrong for a library.
//
// For a private dictionary that belongs to one read instead of to the process,
// build one and pass it in:
//
//	vendor := godicom.NewPrivateDictionary()
//	if err := vendor.Add("ACME 3.2", tag, godicom.VRUS, "Some Number"); err != nil {
//		return err
//	}
//	ds, err := godicom.ReadFile("ct.dcm", &godicom.ReadOptions{
//		Dictionary: godicom.NewDictionary(vendor, godicom.Standard()),
//	})
//
// This function is not deprecated: a command-line tool registering its vendor's
// blocks at startup is exactly what it is for.
func AddPrivateDictEntry(creator string, tag Tag, vr VR, name string, vm ...string) error {
	if !tag.IsPrivate() {
		// Kept verbatim rather than delegated to PrivateDictionary.Add's wording:
		// this message names the function a caller actually called.
		return fmt.Errorf(
			"godicom: non-private tag %s cannot be added with AddPrivateDictEntry",
			tag,
		)
	}
	return extraPrivateDictionary.Add(creator, tag, vr, name, vm...)
}

// ResetExtraPrivateDictionaries clears runtime private dictionary additions.
// Intended for tests.
//
// It clears all of them, including entries registered by code that has nothing
// to do with the caller, because the table it empties is process-global. A test
// that wants a private dictionary only it can see should build one with
// NewPrivateDictionary and pass it through ReadOptions.Dictionary; there is then
// nothing to reset.
func ResetExtraPrivateDictionaries() {
	extraPrivateDictionary.reset()
}
