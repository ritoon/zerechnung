// Copied from github.com/piglig/go-qr v1.1.0 (mask.go, template.go), MIT
// License, Copyright (c) 2023 piglig; see LICENSE.

package qr

import "sync"

// The weights of the penalty rules of ISO/IEC 18004 that choose the mask.
const (
	penaltyN1 = 3  // run of 5 or more modules of the same color
	penaltyN2 = 3  // 2×2 block of the same color
	penaltyN3 = 40 // pattern 1:1:3:1:1 beside 4 light modules
	penaltyN4 = 10 // each 5% of imbalance between dark and light modules
)

// maskInvert reports whether mask pattern msk inverts the module (x, y).
func maskInvert(msk, x, y int) bool {
	switch msk {
	case 0:
		return (x+y)%2 == 0
	case 1:
		return y%2 == 0
	case 2:
		return x%3 == 0
	case 3:
		return (x+y)%3 == 0
	case 4:
		return (x/3+y/2)%2 == 0
	case 5:
		return x*y%2+x*y%3 == 0
	case 6:
		return (x*y%2+x*y%3)%2 == 0
	case 7:
		return ((x+y)%2+x*y%3)%2 == 0
	}
	return false
}

// applyMask applies mask pattern msk to the modules outside the function
// patterns.
func (q *builder) applyMask(msk int) {
	for y := range q.size {
		for x := range q.size {
			invert := maskInvert(msk, x, y)
			q.modules[y][x] = q.modules[y][x] != (invert && !q.isFunction[y][x])
		}
	}
}

// writeMaskedGrid writes into dst the modules XORed with pattern, a mask
// pattern of the template of the version, which is false on the function
// patterns. The grid itself is left unchanged.
func (q *builder) writeMaskedGrid(dst, pattern [][]bool) {
	for y := range q.size {
		row := q.modules[y]
		pat := pattern[y]
		out := dst[y]
		for x := range q.size {
			out[x] = row[x] != pat[x]
		}
	}
}

// penaltyScore returns the penalty of grid by the four rules of ISO/IEC
// 18004.
//
// The score only grows, so the computation stops once it reaches limit, the
// best score so far, and returns a value of at least limit. math.MaxInt32
// gives the exact score.
func (q *builder) penaltyScore(grid [][]bool, limit int) int {
	res := 0
	size := q.size
	var runHistoryArr [7]int
	runHistory := runHistoryArr[:]
	dark := 0

	// Rows: rule 1 (runs), rule 3 (finder-like patterns), rule 2 (2×2
	// blocks, with the next row) and the count of dark modules of rule 4,
	// in one sweep.
	for y := range size {
		row := grid[y]
		var next []bool
		if y+1 < size {
			next = grid[y+1]
		}
		runColor, runX := false, 0
		runHistoryArr = [7]int{}
		for x := range size {
			cell := row[x]
			if cell {
				dark++
			}
			if cell == runColor {
				runX++
				if runX == 5 {
					res += penaltyN1
				} else if runX > 5 {
					res++
				}
			} else {
				q.finderPenaltyAddHistory(runX, runHistory)
				if !runColor {
					res += q.finderPenaltyCountPatterns(runHistory) * penaltyN3
				}
				runColor = cell
				runX = 1
			}
			if next != nil && x+1 < size &&
				cell == row[x+1] && cell == next[x] && cell == next[x+1] {
				res += penaltyN2
			}
		}
		res += q.finderPenaltyTerminateAndCount(runColor, runX, runHistory) * penaltyN3
	}
	if res >= limit {
		return res
	}

	// Columns: rules 1 and 3.
	for x := range size {
		runColor, runY := false, 0
		runHistoryArr = [7]int{}
		for y := range size {
			cell := grid[y][x]
			if cell == runColor {
				runY++
				if runY == 5 {
					res += penaltyN1
				} else if runY > 5 {
					res++
				}
			} else {
				q.finderPenaltyAddHistory(runY, runHistory)
				if !runColor {
					res += q.finderPenaltyCountPatterns(runHistory) * penaltyN3
				}
				runColor = cell
				runY = 1
			}
		}
		res += q.finderPenaltyTerminateAndCount(runColor, runY, runHistory) * penaltyN3
	}
	if res >= limit {
		return res
	}

	// Rule 4: balance of dark and light modules, by steps of 5%.
	total := size * size
	k := (abs(dark*20-total*10)+total-1)/total - 1
	res += k * penaltyN4
	return res
}

// finderPenaltyCountPatterns returns how many 1:1:3:1:1 patterns, with 4
// light modules on one side, end the run history: 0, 1 or 2.
func (q *builder) finderPenaltyCountPatterns(runHistory []int) int {
	n := runHistory[1]
	core := n > 0 && runHistory[2] == n && runHistory[3] == n*3 && runHistory[4] == n && runHistory[5] == n
	res := 0
	if core && runHistory[0] >= n*4 && runHistory[6] >= n {
		res++
	}
	if core && runHistory[6] >= n*4 && runHistory[0] >= n {
		res++
	}
	return res
}

// finderPenaltyTerminateAndCount closes the run history at the end of a row
// or column, the light quiet zone counting as a run, and returns the count
// of finderPenaltyCountPatterns.
func (q *builder) finderPenaltyTerminateAndCount(currentRunColor bool, currentRunLen int, runHistory []int) int {
	if currentRunColor {
		q.finderPenaltyAddHistory(currentRunLen, runHistory)
		currentRunLen = 0
	}
	currentRunLen += q.size
	q.finderPenaltyAddHistory(currentRunLen, runHistory)
	return q.finderPenaltyCountPatterns(runHistory)
}

// finderPenaltyAddHistory pushes the length of a run in front of the run
// history; the first light run of a row or column takes in the quiet zone.
func (q *builder) finderPenaltyAddHistory(currentRunLen int, runHistory []int) {
	if runHistory[0] == 0 {
		currentRunLen += q.size
	}
	runHistory[6] = runHistory[5]
	runHistory[5] = runHistory[4]
	runHistory[4] = runHistory[3]
	runHistory[3] = runHistory[2]
	runHistory[2] = runHistory[1]
	runHistory[1] = runHistory[0]
	runHistory[0] = currentRunLen
}

// template holds the eight mask patterns of a version: maskPatterns[m][y][x]
// reports whether mask m inverts the module (x, y), always false on the
// function patterns. It depends on the version only.
type template struct {
	maskPatterns [8][][]bool
}

// templateCache maps a version to its *template, immutable once built.
var templateCache sync.Map

// getTemplate returns the template of version ver, built on first use.
func getTemplate(ver int) *template {
	if v, ok := templateCache.Load(ver); ok {
		return v.(*template)
	}
	size := ver*4 + 17
	b := newBuilder(ver, Low)
	b.drawFunctionPatterns()
	t := &template{}
	for m := range 8 {
		grid := make([][]bool, size)
		backing := make([]bool, size*size)
		for y := range size {
			grid[y] = backing[y*size : (y+1)*size]
			fn := b.isFunction[y]
			for x := range size {
				grid[y][x] = maskInvert(m, x, y) && !fn[x]
			}
		}
		t.maskPatterns[m] = grid
	}
	actual, _ := templateCache.LoadOrStore(ver, t)
	return actual.(*template)
}
