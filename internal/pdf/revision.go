package pdf

import "slices"

// UpdatedAfter returns the numbers of the objects that the cross-reference
// sections written at or after offset off define, in increasing order: the
// objects that the incremental updates appended after the revision ending
// at off add, replace or free. ok is false when the cross-reference data
// had to be rebuilt by scanning, which tells nothing about revisions.
func (r *Reader) UpdatedAfter(off int64) (nums []uint32, ok bool) {
	if r.recOnly || len(r.sections) == 0 {
		return nil, false
	}
	seen := map[uint32]bool{}
	add := func(n uint32) {
		if n != 0 && !seen[n] {
			seen[n] = true
			nums = append(nums, n)
		}
	}
	var visit func(s *section)
	visit = func(s *section) {
		for _, ss := range s.subs {
			for i := uint32(0); i < ss.count && i < 1<<23; i++ {
				add(ss.first + i)
			}
		}
		for n := range s.lenient {
			add(n)
		}
		if s.xs != nil {
			for i := 0; i+1 < len(s.xs.index); i += 2 {
				for k := uint32(0); k < s.xs.index[i+1] && k < 1<<23; k++ {
					add(s.xs.index[i] + k)
				}
			}
		}
		if s.hybrid != nil {
			visit(s.hybrid)
		}
	}
	for _, s := range r.sections {
		if s.off >= off {
			visit(s)
		}
	}
	slices.Sort(nums)
	return nums, true
}
