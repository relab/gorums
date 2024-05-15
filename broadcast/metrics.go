package broadcast

type Metrics struct {
	TotalNum     uint64
	FinishedReqs struct {
		Total     uint64
		Succesful uint64
		Failed    uint64
	}
	Dropped uint64
}
