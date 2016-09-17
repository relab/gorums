package gridq

import "sort"

// NewGQSort returns a gird quorum specification (sort version).
func NewGQSort(rows, cols int, printGrid bool) QuorumSpec {
	return &gqSort{
		rows:      rows,
		cols:      cols,
		printGrid: printGrid,
		vgrid:     newVisualGrid(rows, cols),
	}
}

type gqSort struct {
	rows, cols int
	printGrid  bool
	vgrid      *visualGrid
}

// ReadQF: All replicas from one row.
func (gqs *gqSort) ReadQF(replies []*ReadResponse) (*ReadResponse, bool) {
	if len(replies) < gqs.rows {
		return nil, false
	}

	sort.Sort(byRowTimestamp(replies))

	qreplies := 1 // Counter for replies from the same row.
	row := replies[0].Row
	for i := 1; i < len(replies); i++ {
		if replies[i].Row != row {
			qreplies = 1
			row = replies[i].Row
			left := len(replies) - i - 1
			if qreplies+left < gqs.rows {
				// Not enough replies left.
				return nil, false
			}
			continue
		}
		qreplies++
		if qreplies == gqs.rows {
			if gqs.printGrid {
				gqs.vgrid.SetRowQuorum(row)
				gqs.vgrid.print()
			}
			return replies[i-gqs.rows+1], true
		}
	}

	panic("an invariant was not handled")
}

// WriteQF: One replica from each row.
func (gqs *gqSort) WriteQF(replies []*WriteResponse) (*WriteResponse, bool) {
	if len(replies) < gqs.cols {
		return nil, false
	}

	sort.Sort(byColRow(replies))

	qreplies := 1 // Counter for replies from the same row.
	col := replies[0].Col
	for i := 1; i < len(replies); i++ {
		if replies[i].Col != col {
			qreplies = 1
			col = replies[i].Col
			left := len(replies) - i - 1
			if qreplies+left < gqs.cols {
				// Not enough replies left.
				return nil, false
			}
			continue
		}
		qreplies++
		if qreplies == gqs.cols {
			if gqs.printGrid {
				gqs.vgrid.SetColQuorum(col)
				gqs.vgrid.print()
			}
			// WriteResponses don't have timestamps.
			return replies[i], true
		}
	}

	panic("an invariant was not handled")
}

// NewGQMap returns a gird quorum specification (map version).
func NewGQMap(rows, cols int, printGrid bool) QuorumSpec {
	return &gqMap{
		rows:      rows,
		cols:      cols,
		printGrid: printGrid,
		vgrid:     newVisualGrid(rows, cols),
	}
}

type gqMap struct {
	rows, cols int
	printGrid  bool
	vgrid      *visualGrid
}

// ReadQF: All replicas from one row.
//
// Note: It is not enough to just know that we have a quorum from a row, we also
// need to know what replies forms the quorum (both in practice and to be fair
// to GQSort above).
func (gqm *gqMap) ReadQF(replies []*ReadResponse) (*ReadResponse, bool) {
	if len(replies) < gqm.rows {
		return nil, false
	}

	rowReplies := make(map[uint32][]*ReadResponse)
	var row []*ReadResponse
	var found bool
	for _, reply := range replies {
		row, found = rowReplies[reply.Row]
		if !found {
			row = make([]*ReadResponse, 0, gqm.rows)
		}
		row = append(row, reply)
		if len(row) >= gqm.rows {
			if gqm.printGrid {
				gqm.vgrid.SetRowQuorum(reply.Row)
				gqm.vgrid.print()
			}
			sort.Sort(byTimestamp(row))
			return row[len(row)-1], true
		}
		rowReplies[reply.Row] = row
	}

	return nil, false
}

// WriteQF: One replica from each row.
func (gqm *gqMap) WriteQF(replies []*WriteResponse) (*WriteResponse, bool) {
	panic("not implemented, symmetric with read")
}

// NewGQSliceOne returns a gird quorum specification (slice version I).
func NewGQSliceOne(rows, cols int, printGrid bool) QuorumSpec {
	return &gqSliceOne{
		rows:      rows,
		cols:      cols,
		printGrid: printGrid,
		vgrid:     newVisualGrid(rows, cols),
	}
}

type gqSliceOne struct {
	rows, cols int
	printGrid  bool
	vgrid      *visualGrid
}

// ReadQF: All replicas from one row.
func (gqs *gqSliceOne) ReadQF(replies []*ReadResponse) (*ReadResponse, bool) {
	if len(replies) < gqs.rows {
		return nil, false
	}
	rowCount := make([]int, gqs.rows)
	repliesRM := make([]*ReadResponse, gqs.rows*gqs.cols) // row-major
	for _, reply := range replies {
		repliesRM[(int(reply.Row)*gqs.rows)+int(reply.Col)] = reply
		rowCount[reply.Row]++
		if rowCount[reply.Row] >= gqs.rows {
			start := int(reply.Row) * gqs.rows
			repliesRM = repliesRM[start : start+gqs.rows]
			sort.Sort(byTimestamp(repliesRM))
			return repliesRM[len(repliesRM)-1], true
		}
	}

	return nil, false
}

// WriteQF: One replica from each row.
func (gqs *gqSliceOne) WriteQF(replies []*WriteResponse) (*WriteResponse, bool) {
	panic("not implemented, symmetric with read")
}

// NewGQSliceTwo returns a gird quorum specification (slice version II).
func NewGQSliceTwo(rows, cols int, printGrid bool) QuorumSpec {
	return &gqSliceTwo{
		rows:      rows,
		cols:      cols,
		printGrid: printGrid,
		vgrid:     newVisualGrid(rows, cols),
	}
}

type gqSliceTwo struct {
	rows, cols int
	printGrid  bool
	vgrid      *visualGrid
}

type rowInfo struct {
	count int           // Replies for this row.
	reply *ReadResponse // Reply with highest timestamp.
}

// ReadQF: All replicas from one row.
func (gqs *gqSliceTwo) ReadQF(replies []*ReadResponse) (*ReadResponse, bool) {
	if len(replies) < gqs.rows {
		return nil, false
	}
	rows := make([]*rowInfo, gqs.rows)
	for _, reply := range replies {
		ri := rows[int(reply.Row)]
		if rows[int(reply.Row)] == nil {
			ri = &rowInfo{}
			rows[int(reply.Row)] = ri
		}
		if ri.reply == nil {
			ri.reply = reply
		} else if reply.State.Timestamp > ri.reply.State.Timestamp {
			ri.reply = reply
		}
		ri.count++
		if ri.count >= gqs.rows {
			if gqs.printGrid {
				gqs.vgrid.SetRowQuorum(reply.Row)
				gqs.vgrid.print()
			}
			return ri.reply, true
		}
	}

	return nil, false
}

// WriteQF: One replica from each row.
func (gqs *gqSliceTwo) WriteQF(replies []*WriteResponse) (*WriteResponse, bool) {
	panic("not implemented, symmetric with read")
}
