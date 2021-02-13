package gorums

func GetQSpec(opts ...ConfigOption) interface{} {
	o := newConfigOptions()
	for _, opt := range opts {
		opt(&o)
	}
	return o.qspec
}
