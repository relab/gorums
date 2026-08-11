package benchkit

import "cmp"

// Dimensions identifies one benchmark configuration across the sweep
// pipeline. Zero buffer sizes select the benchmark binary's defaults.
type Dimensions struct {
	Benchmark  string `json:"benchmark"`
	Nodes      int    `json:"nodes"`
	Workers    int    `json:"workers"`
	Payload    int    `json:"payload"`
	Rate       int    `json:"rate"`
	SendBuffer int    `json:"send_buffer"`
	RecvBuffer int    `json:"recv_buffer"`
	StreamMode string `json:"stream_mode"`
}

// Dimensions returns the sweep dimensions recorded in cfg.
func (cfg *RunConfig) Dimensions() Dimensions {
	if cfg == nil {
		return Dimensions{}
	}
	return Dimensions{
		Benchmark:  cfg.GetName(),
		Nodes:      int(cfg.GetNumNodes()),
		Workers:    int(cfg.GetWorkers()),
		Payload:    int(cfg.GetPayload()),
		Rate:       int(cfg.GetRate()),
		SendBuffer: int(cfg.GetSendBuffer()),
		RecvBuffer: int(cfg.GetRecvBuffer()),
		StreamMode: cfg.GetStreamMode(),
	}
}

// DimensionsWithFallback returns the dimensions recorded in cfg, filling
// zero-valued fields from fallback for results written before those fields
// were recorded.
func (cfg *RunConfig) DimensionsWithFallback(fallback Dimensions) Dimensions {
	dims := cfg.Dimensions()
	dims.Benchmark = cmp.Or(dims.Benchmark, fallback.Benchmark)
	dims.Nodes = cmp.Or(dims.Nodes, fallback.Nodes)
	dims.Workers = cmp.Or(dims.Workers, fallback.Workers)
	dims.Payload = cmp.Or(dims.Payload, fallback.Payload)
	dims.Rate = cmp.Or(dims.Rate, fallback.Rate)
	dims.SendBuffer = cmp.Or(dims.SendBuffer, fallback.SendBuffer)
	dims.RecvBuffer = cmp.Or(dims.RecvBuffer, fallback.RecvBuffer)
	dims.StreamMode = cmp.Or(dims.StreamMode, fallback.StreamMode)
	return dims
}

// ApplyDimensions records d in cfg.
func (cfg *RunConfig) ApplyDimensions(d Dimensions) {
	cfg.SetName(d.Benchmark)
	cfg.SetNumNodes(int32(d.Nodes))
	cfg.SetWorkers(int32(d.Workers))
	cfg.SetPayload(int32(d.Payload))
	cfg.SetRate(int64(d.Rate))
	cfg.SetSendBuffer(int32(d.SendBuffer))
	cfg.SetRecvBuffer(int32(d.RecvBuffer))
	cfg.SetStreamMode(d.StreamMode)
}

// NewRunConfig returns a run configuration containing d.
func NewRunConfig(d Dimensions) *RunConfig {
	cfg := RunConfig_builder{}.Build()
	cfg.ApplyDimensions(d)
	return cfg
}
