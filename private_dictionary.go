package godicom

import (
	"fmt"
	"sync"
)

var extraPrivateDictionaries = map[string]map[string]PrivateDictEntry{}
var extraPrivateMu sync.RWMutex

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

func lookupPrivateDictEntry(tag Tag, creator string) (PrivateDictEntry, bool) {
	keys := privateTagKeys(tag)
	lookupIn := func(dict map[string]map[string]PrivateDictEntry) (PrivateDictEntry, bool) {
		inner, ok := dict[creator]
		if !ok {
			return PrivateDictEntry{}, false
		}
		for _, key := range keys {
			if entry, ok := inner[key]; ok {
				return entry, true
			}
		}
		return PrivateDictEntry{}, false
	}

	if entry, ok := lookupIn(PrivateDictionaries); ok {
		return entry, true
	}

	extraPrivateMu.RLock()
	defer extraPrivateMu.RUnlock()
	return lookupIn(extraPrivateDictionaries)
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

// AddPrivateDictEntry adds or updates a runtime private dictionary entry.
func AddPrivateDictEntry(creator string, tag Tag, vr VR, name string, vm ...string) error {
	multiplicity := "1"
	if len(vm) > 0 {
		multiplicity = vm[0]
	}
	if !tag.IsPrivate() {
		return fmt.Errorf(
			"godicom: non-private tag %s cannot be added with AddPrivateDictEntry",
			tag,
		)
	}

	key := fmt.Sprintf("%04Xxx%02X", tag.Group(), tag.Element()&0xFF)
	entry := PrivateDictEntry{
		VR:   string(vr),
		VM:   multiplicity,
		Name: name,
	}

	extraPrivateMu.Lock()
	defer extraPrivateMu.Unlock()
	if _, ok := extraPrivateDictionaries[creator]; !ok {
		extraPrivateDictionaries[creator] = map[string]PrivateDictEntry{}
	}
	extraPrivateDictionaries[creator][key] = entry
	return nil
}

// ResetExtraPrivateDictionaries clears runtime private dictionary additions.
// Intended for tests.
func ResetExtraPrivateDictionaries() {
	extraPrivateMu.Lock()
	defer extraPrivateMu.Unlock()
	extraPrivateDictionaries = map[string]map[string]PrivateDictEntry{}
}
