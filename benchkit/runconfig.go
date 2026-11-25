package benchkit

import (
	"cmp"
	"encoding/json"
	"iter"
	"maps"
	"net/url"
	"slices"
)

// Dimensions identifies one benchmark configuration across the sweep
// pipeline. Zero buffer sizes select the benchmark binary's defaults. Params
// holds the values of the custom dimensions a sweep declares.
type Dimensions struct {
	Benchmark  string `json:"benchmark"`
	Nodes      int    `json:"nodes"`
	Workers    int    `json:"workers"`
	Payload    int    `json:"payload"`
	Rate       int    `json:"rate"`
	SendBuffer int    `json:"send_buffer"`
	RecvBuffer int    `json:"recv_buffer"`
	StreamMode string `json:"stream_mode"`
	Params     Params `json:"params,omitempty"`
}

// Params holds the values of the custom sweep dimensions of one
// configuration, keyed by dimension name. It is a canonical string: the
// query-escaped name=value pairs sorted by name and joined by "&". Equal sets
// of values give equal strings, so a [Dimensions] holding them stays
// comparable. The zero value holds no values. In JSON, Params is an object
// that maps each name to its value.
type Params string

// NewParams returns the Params holding values.
func NewParams(values map[string]string) Params {
	query := make(url.Values, len(values))
	for name, value := range values {
		query.Set(name, value)
	}
	return Params(query.Encode())
}

// query decodes p. A Params built by [NewParams] always decodes; a malformed
// string decodes to the pairs that parse.
func (p Params) query() url.Values {
	query, _ := url.ParseQuery(string(p))
	return query
}

// Get returns the value of the named dimension, or "" when p has no value for it.
func (p Params) Get(name string) string {
	return p.query().Get(name)
}

// All returns an iterator over the name-value pairs of p, sorted by name.
func (p Params) All() iter.Seq2[string, string] {
	return func(yield func(string, string) bool) {
		query := p.query()
		for _, name := range slices.Sorted(maps.Keys(query)) {
			if !yield(name, query.Get(name)) {
				return
			}
		}
	}
}

// Map returns the name-value pairs of p, or nil when p holds none.
func (p Params) Map() map[string]string {
	if p == "" {
		return nil
	}
	return maps.Collect(p.All())
}

// Names returns the dimension names in p, sorted.
func (p Params) Names() []string {
	return slices.Sorted(maps.Keys(p.query()))
}

// MarshalJSON encodes p as a JSON object that maps each name to its value.
func (p Params) MarshalJSON() ([]byte, error) {
	values := p.Map()
	if values == nil {
		values = map[string]string{}
	}
	return json.Marshal(values)
}

// UnmarshalJSON decodes a JSON object that maps each name to its value.
func (p *Params) UnmarshalJSON(data []byte) error {
	var values map[string]string
	if err := json.Unmarshal(data, &values); err != nil {
		return err
	}
	*p = NewParams(values)
	return nil
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
		Params:     NewParams(cfg.GetParams()),
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
	dims.Params = cmp.Or(dims.Params, fallback.Params)
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
	cfg.SetParams(d.Params.Map())
}

// NewRunConfig returns a run configuration containing d.
func NewRunConfig(d Dimensions) *RunConfig {
	cfg := RunConfig_builder{}.Build()
	cfg.ApplyDimensions(d)
	return cfg
}
