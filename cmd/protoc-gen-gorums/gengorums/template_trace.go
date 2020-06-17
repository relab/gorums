package gengorums

import (
	"fmt"

	"github.com/relab/gorums"
	"google.golang.org/protobuf/compiler/protogen"
)

var traceable bool

// SetTrace is used to indicate whether or not to generate trace code.
func SetTrace(trace bool) {
	traceable = trace
}

func trace() string {
	if traceable {
		return traceDefinition
	}
	return notrace
}

var traceDefinition = `
{{define "trace"}}
	var ti traceInfo
	if c.mgr.opts.trace {
		ti.Trace = {{use "trace.New" .GenFile}}("gorums."+c.tstring()+".Sent", "{{.Method.GoName}}")
		defer ti.Finish()

		ti.firstLine.cid = c.id
		if deadline, ok := ctx.Deadline(); ok {
			ti.firstLine.deadline = {{use "time.Until" .GenFile}}(deadline)
		}
		ti.LazyLog(&ti.firstLine, false)
		ti.LazyLog(&payload{sent: true, msg: in}, false)

		defer func() {
			ti.LazyLog({{qcresult .GenFile .Method}}, false)
			if {{withPromise .Method "resp."}}err != nil {
				ti.SetError()
			}
		}()
	}
{{end}}

{{define "traceLazyLog"}}
	if c.mgr.opts.trace {
		ti.LazyLog(&payload{sent: false, id: r.nid, msg: r.reply}, false)
	}
{{end}}
`

var notrace = `
{{define "trace"}}{{end}}
{{define "traceLazyLog"}}{{end}}
`

func qcresult(g *protogen.GeneratedFile, method *protogen.Method) string {
	if hasMethodOption(method, callTypesWithPromiseObject...) {
		return fmt.Sprintf("&qcresult{ids: resp.NodeIDs, reply: resp.%s, err: resp.err}", field(customOut(g, method)))
	}
	if hasMethodOption(method, gorums.E_Quorumcall, gorums.E_Ordered) {
		return "&qcresult{reply: resp, err: err}"
	}
	return ""
}
