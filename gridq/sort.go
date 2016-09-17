package gridq

type ByRowCol []*ReadResponse

func (p ByRowCol) Len() int      { return len(p) }
func (p ByRowCol) Swap(i, j int) { p[i], p[j] = p[j], p[i] }
func (p ByRowCol) Less(i, j int) bool {
	if p[i].Row < p[j].Row {
		return true
	} else if p[i].Row > p[j].Row {
		return false
	} else {
		return p[i].Col < p[j].Col
	}
}

type ByColRow []*WriteResponse

func (p ByColRow) Len() int      { return len(p) }
func (p ByColRow) Swap(i, j int) { p[i], p[j] = p[j], p[i] }
func (p ByColRow) Less(i, j int) bool {
	if p[i].Col < p[j].Col {
		return true
	} else if p[i].Col > p[j].Col {
		return false
	} else {
		return p[i].Row < p[j].Row
	}
}

type ByRowTimestamp []*ReadResponse

func (p ByRowTimestamp) Len() int      { return len(p) }
func (p ByRowTimestamp) Swap(i, j int) { p[i], p[j] = p[j], p[i] }
func (p ByRowTimestamp) Less(i, j int) bool {
	if p[i].Row < p[j].Row {
		return true
	} else if p[i].Row > p[j].Row {
		return false
	} else {
		return p[i].State.Timestamp > p[j].State.Timestamp
	}
}

type ByTimestamp []*ReadResponse

func (p ByTimestamp) Len() int           { return len(p) }
func (p ByTimestamp) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }
func (p ByTimestamp) Less(i, j int) bool { return p[i].State.Timestamp < p[j].State.Timestamp }
