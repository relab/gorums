package gridq

import (
	"fmt"
	"sync"
)

type visualGrid struct {
	sync.Mutex
	grid       [][]rune
	crow, ccol int
}

func newVisualGrid(rows, cols int) *visualGrid {
	grid := make([][]rune, rows)
	cells := make([]rune, rows*cols)
	for i := range grid {
		grid[i], cells = cells[:cols], cells[cols:]
	}

	vgrid := &visualGrid{
		grid: grid,
		crow: -1,
		ccol: -1,
	}

	return vgrid
}

func (vg *visualGrid) SetRowQuorum(row uint32) {
	vg.Lock()
	defer vg.Unlock()
	if int(row) == vg.crow {
		return
	}
	vg.crow, vg.ccol = int(row), -1
	val := '-'
	for i := range vg.grid {
		if i == int(row) {
			val = 'Q'
		}
		for j := range vg.grid[i] {
			vg.grid[i][j] = val
		}
		val = '-'
	}
}

func (vg *visualGrid) SetColQuorum(col uint32) {
	vg.Lock()
	defer vg.Unlock()
	if int(col) == vg.ccol {
		return
	}
	vg.crow, vg.ccol = -1, int(col)
	for i := range vg.grid {
		for j := range vg.grid[i] {
			if j == int(col) {
				vg.grid[i][j] = 'Q'
			} else {
				vg.grid[i][j] = '-'
			}
		}
	}
}

func (vg *visualGrid) print() {
	vg.Lock()
	defer vg.Unlock()
	if vg.crow != -1 {
		fmt.Println("row quorum:")
	} else if vg.ccol != -1 {
		fmt.Println("col quorum:")
	} else {
		fmt.Println("no quorum:")
	}
	for i := range vg.grid {
		fmt.Printf("%c\n", vg.grid[i])
	}
}
