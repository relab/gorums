package gengorums

var globals = `
const hasStrictOrderingMethods = {{hasStrictOrderingMethods .Services}}
`

var internalOutDataType = `
{{range $intOut, $out := mapInternalOutType .GenFile .Services}}
type {{$intOut}} struct {
	nid   uint32
	reply *{{$out}}
	err   error
}
{{end}}
`

// This struct and API functions are generated only once per return type
// for a future call type. That is, if multiple future calls use the same
// return type, this struct and associated methods are only generated once.
var futureDataType = `
{{range $futureOut, $customOut := mapFutureOutType .GenFile .Services}}
{{$customOutField := field $customOut}}
// {{$futureOut}} is a future object for processing replies.
type {{$futureOut}} struct {
	// the actual reply
	*{{$customOut}}
	NodeIDs  []uint32
	err      error
	c        chan struct{}
}

// Get returns the reply and any error associated with the called method.
// The method blocks until a reply or error is available.
func (f *{{$futureOut}}) Get() (*{{$customOut}}, error) {
	<-f.c
	return f.{{$customOutField}}, f.err
}

// Done reports if a reply and/or error is available for the called method.
func (f *{{$futureOut}}) Done() bool {
	select {
	case <-f.c:
		return true
	default:
		return false
	}
}
{{end}}
`

// This struct and API functions are generated only once per return type
// for a correctable call type. That is, if multiple correctable calls use the same
// return type, this struct and associated methods are only generated once.
var correctableDataType = `
{{$genFile := .GenFile}}
{{range $correctableOut, $customOut := mapCorrectableOutType .GenFile .Services}}
{{$customOutField := field $customOut}}
// {{$correctableOut}} is a correctable object for processing replies.
type {{$correctableOut}} struct {
	mu {{use "sync.Mutex" $genFile}}
	// the actual reply
	*{{$customOut}}
	NodeIDs  []uint32
	level    int
	err      error
	done     bool
	watchers []*struct {
		level	int
		ch		chan struct{}
	}
	donech chan struct{}
}

// Get returns the reply, level and any error associated with the
// called method. The method does not block until a (possibly
// itermidiate) reply or error is available. Level is set to LevelNotSet if no
// reply has yet been received. The Done or Watch methods should be used to
// ensure that a reply is available.
func (c *{{$correctableOut}}) Get() (*{{$customOut}}, int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.{{$customOutField}}, c.level, c.err
}

// Done returns a channel that will be closed when the correctable
// quorum call is done. A call is considered done when the quorum function has
// signaled that a quorum of replies was received or the call returned an error.
func (c *{{$correctableOut}}) Done() <-chan struct{} {
	return c.donech
}

// Watch returns a channel that will be closed when a reply or error at or above the
// specified level is available. If the call is done, the channel is closed
// regardless of the specified level.
func (c *{{$correctableOut}}) Watch(level int) <-chan struct{} {
	ch := make(chan struct{})
	c.mu.Lock()
	defer c.mu.Unlock()
	if level < c.level {
		close(ch)
		return ch
	}
	c.watchers = append(c.watchers, &struct {
		level int
		ch    chan struct{}
	}{level, ch})
	return ch
}

func (c *{{$correctableOut}}) set(reply *{{$customOut}}, level int, err error, done bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.done {
		panic("set(...) called on a done correctable")
	}
	c.{{$customOutField}}, c.level, c.err, c.done = reply, level, err, done
	if done {
		close(c.donech)
		for _, watcher := range c.watchers {
			if watcher != nil {
				close(watcher.ch)
			}
		}
		return
	}
	for i := range c.watchers {
		if c.watchers[i] != nil && c.watchers[i].level <= level {
			close(c.watchers[i].ch)
			c.watchers[i] = nil
		}
	}
}
{{end}}
`

var datatypes = globals +
	internalOutDataType +
	futureDataType +
	correctableDataType
