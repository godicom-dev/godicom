package godicom

// MultiValue holds a DICOM multi-valued element.
// All items must be of the same type.
type MultiValue[T any] struct {
	values []T
}

func NewMultiValue[T any](items []T) *MultiValue[T] {
	return &MultiValue[T]{values: items}
}

func (mv *MultiValue[T]) Values() []T    { return mv.values }
func (mv *MultiValue[T]) Len() int       { return len(mv.values) }
func (mv *MultiValue[T]) Get(i int) T    { return mv.values[i] }
func (mv *MultiValue[T]) Set(i int, v T) { mv.values[i] = v }
func (mv *MultiValue[T]) Append(v T)     { mv.values = append(mv.values, v) }
func (mv *MultiValue[T]) IsEmpty() bool  { return len(mv.values) == 0 }
