package gridq

type byColRow []*WriteResponse

func (p byColRow) Len() int      { return len(p) }
func (p byColRow) Swap(i, j int) { p[i], p[j] = p[j], p[i] }
func (p byColRow) Less(i, j int) bool {
	if p[i].Col < p[j].Col {
		return true
	} else if p[i].Col > p[j].Col {
		return false
	} else {
		return p[i].Row < p[j].Row
	}
}

type byRowTimestamp []*ReadResponse

func (p byRowTimestamp) Len() int      { return len(p) }
func (p byRowTimestamp) Swap(i, j int) { p[i], p[j] = p[j], p[i] }
func (p byRowTimestamp) Less(i, j int) bool {
	if p[i].Row < p[j].Row {
		return true
	} else if p[i].Row > p[j].Row {
		return false
	} else {
		return p[i].State.Timestamp > p[j].State.Timestamp
	}
}

type byTimestamp []*ReadResponse

func (p byTimestamp) Len() int           { return len(p) }
func (p byTimestamp) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }
func (p byTimestamp) Less(i, j int) bool { return p[i].State.Timestamp < p[j].State.Timestamp }
