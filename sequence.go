package godicom

// Sequence holds a list of Dataset items (for VR SQ).
type Sequence struct {
	items             []*Dataset
	IsUndefinedLength bool
	owner             *Dataset // dataset that directly contains this sequence element
}

func NewSequence(items []*Dataset) *Sequence {
	return &Sequence{items: items}
}

func (s *Sequence) Items() []*Dataset  { return s.items }
func (s *Sequence) Len() int           { return len(s.items) }
func (s *Sequence) Get(i int) *Dataset { return s.items[i] }
func (s *Sequence) Append(ds *Dataset) {
	if ds != nil {
		ds.parent = s
	}
	s.items = append(s.items, ds)
}
func (s *Sequence) IsEmpty() bool { return len(s.items) == 0 }
