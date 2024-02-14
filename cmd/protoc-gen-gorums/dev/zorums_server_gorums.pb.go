// Code generated by protoc-gen-gorums. DO NOT EDIT.
// versions:
// 	protoc-gen-gorums v0.7.0-devel
<<<<<<< HEAD
// 	protoc            v4.24.4
=======
// 	protoc            v3.12.4
>>>>>>> 0da29c48 (init code gen)
// source: zorums.proto

package dev

import (
	empty "github.com/golang/protobuf/ptypes/empty"
	gorums "github.com/relab/gorums"
	ordering "github.com/relab/gorums/ordering"
	proto "google.golang.org/protobuf/proto"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = gorums.EnforceVersion(7 - gorums.MinVersion)
	// Verify that the gorums runtime is sufficiently up-to-date.
	_ = gorums.EnforceVersion(gorums.MaxVersion - 7)
)

// ZorumsService is the server-side API for the ZorumsService Service
type ZorumsService interface {
	GRPCCall(ctx gorums.ServerCtx, request *Request) (response *Response, err error)
	QuorumCall(ctx gorums.ServerCtx, request *Request) (response *Response, err error)
	QuorumCallPerNodeArg(ctx gorums.ServerCtx, request *Request) (response *Response, err error)
	QuorumCallCustomReturnType(ctx gorums.ServerCtx, request *Request) (response *Response, err error)
	QuorumCallCombo(ctx gorums.ServerCtx, request *Request) (response *Response, err error)
	QuorumCallEmpty(ctx gorums.ServerCtx, request *empty.Empty) (response *Response, err error)
	QuorumCallEmpty2(ctx gorums.ServerCtx, request *Request) (response *empty.Empty, err error)
	Multiparty(ctx gorums.ServerCtx, request *Request, broadcast *Broadcast)
	MultipartyInternal(ctx gorums.ServerCtx, request *Request, broadcast *Broadcast)
	Multicast(ctx gorums.ServerCtx, request *Request)
	MulticastPerNodeArg(ctx gorums.ServerCtx, request *Request)
	Multicast2(ctx gorums.ServerCtx, request *Request)
	Multicast3(ctx gorums.ServerCtx, request *Request)
	Multicast4(ctx gorums.ServerCtx, request *empty.Empty)
	QuorumCallAsync(ctx gorums.ServerCtx, request *Request) (response *Response, err error)
	QuorumCallAsyncPerNodeArg(ctx gorums.ServerCtx, request *Request) (response *Response, err error)
	QuorumCallAsyncCustomReturnType(ctx gorums.ServerCtx, request *Request) (response *Response, err error)
	QuorumCallAsyncCombo(ctx gorums.ServerCtx, request *Request) (response *Response, err error)
	QuorumCallAsync2(ctx gorums.ServerCtx, request *Request) (response *Response, err error)
	QuorumCallAsyncEmpty(ctx gorums.ServerCtx, request *Request) (response *empty.Empty, err error)
	QuorumCallAsyncEmpty2(ctx gorums.ServerCtx, request *empty.Empty) (response *Response, err error)
	Correctable(ctx gorums.ServerCtx, request *Request) (response *Response, err error)
	CorrectablePerNodeArg(ctx gorums.ServerCtx, request *Request) (response *Response, err error)
	CorrectableCustomReturnType(ctx gorums.ServerCtx, request *Request) (response *Response, err error)
	CorrectableCombo(ctx gorums.ServerCtx, request *Request) (response *Response, err error)
	CorrectableEmpty(ctx gorums.ServerCtx, request *Request) (response *empty.Empty, err error)
	CorrectableEmpty2(ctx gorums.ServerCtx, request *empty.Empty) (response *Response, err error)
	CorrectableStream(ctx gorums.ServerCtx, request *Request, send func(response *Response) error) error
	CorrectableStreamPerNodeArg(ctx gorums.ServerCtx, request *Request, send func(response *Response) error) error
	CorrectableStreamCustomReturnType(ctx gorums.ServerCtx, request *Request, send func(response *Response) error) error
	CorrectableStreamCombo(ctx gorums.ServerCtx, request *Request, send func(response *Response) error) error
	CorrectableStreamEmpty(ctx gorums.ServerCtx, request *Request, send func(response *empty.Empty) error) error
	CorrectableStreamEmpty2(ctx gorums.ServerCtx, request *empty.Empty, send func(response *Response) error) error
	Unicast(ctx gorums.ServerCtx, request *Request)
	Unicast2(ctx gorums.ServerCtx, request *Request)
}

func (srv *Server) GRPCCall(ctx gorums.ServerCtx, request *Request) (response *Response, err error) {
	panic("GRPCCall not implemented")
}
func (srv *Server) QuorumCall(ctx gorums.ServerCtx, request *Request) (response *Response, err error) {
	panic("QuorumCall not implemented")
}
func (srv *Server) QuorumCallPerNodeArg(ctx gorums.ServerCtx, request *Request) (response *Response, err error) {
	panic("QuorumCallPerNodeArg not implemented")
}
func (srv *Server) QuorumCallCustomReturnType(ctx gorums.ServerCtx, request *Request) (response *Response, err error) {
	panic("QuorumCallCustomReturnType not implemented")
}
func (srv *Server) QuorumCallCombo(ctx gorums.ServerCtx, request *Request) (response *Response, err error) {
	panic("QuorumCallCombo not implemented")
}
func (srv *Server) QuorumCallEmpty(ctx gorums.ServerCtx, request *empty.Empty) (response *Response, err error) {
	panic("QuorumCallEmpty not implemented")
}
func (srv *Server) QuorumCallEmpty2(ctx gorums.ServerCtx, request *Request) (response *empty.Empty, err error) {
	panic("QuorumCallEmpty2 not implemented")
}
func (srv *Server) Multiparty(ctx gorums.ServerCtx, request *Request, broadcast *Broadcast) {
	panic("Multiparty not implemented")
}
func (srv *Server) MultipartyInternal(ctx gorums.ServerCtx, request *Request, broadcast *Broadcast) {
	panic("MultipartyInternal not implemented")
}
func (srv *Server) Multicast(ctx gorums.ServerCtx, request *Request) {
	panic("Multicast not implemented")
}
func (srv *Server) MulticastPerNodeArg(ctx gorums.ServerCtx, request *Request) {
	panic("MulticastPerNodeArg not implemented")
}
func (srv *Server) Multicast2(ctx gorums.ServerCtx, request *Request) {
	panic("Multicast2 not implemented")
}
func (srv *Server) Multicast3(ctx gorums.ServerCtx, request *Request) {
	panic("Multicast3 not implemented")
}
func (srv *Server) Multicast4(ctx gorums.ServerCtx, request *empty.Empty) {
	panic("Multicast4 not implemented")
}
func (srv *Server) QuorumCallAsync(ctx gorums.ServerCtx, request *Request) (response *Response, err error) {
	panic("QuorumCallAsync not implemented")
}
func (srv *Server) QuorumCallAsyncPerNodeArg(ctx gorums.ServerCtx, request *Request) (response *Response, err error) {
	panic("QuorumCallAsyncPerNodeArg not implemented")
}
func (srv *Server) QuorumCallAsyncCustomReturnType(ctx gorums.ServerCtx, request *Request) (response *Response, err error) {
	panic("QuorumCallAsyncCustomReturnType not implemented")
}
func (srv *Server) QuorumCallAsyncCombo(ctx gorums.ServerCtx, request *Request) (response *Response, err error) {
	panic("QuorumCallAsyncCombo not implemented")
}
func (srv *Server) QuorumCallAsync2(ctx gorums.ServerCtx, request *Request) (response *Response, err error) {
	panic("QuorumCallAsync2 not implemented")
}
func (srv *Server) QuorumCallAsyncEmpty(ctx gorums.ServerCtx, request *Request) (response *empty.Empty, err error) {
	panic("QuorumCallAsyncEmpty not implemented")
}
func (srv *Server) QuorumCallAsyncEmpty2(ctx gorums.ServerCtx, request *empty.Empty) (response *Response, err error) {
	panic("QuorumCallAsyncEmpty2 not implemented")
}
func (srv *Server) Correctable(ctx gorums.ServerCtx, request *Request) (response *Response, err error) {
	panic("Correctable not implemented")
}
func (srv *Server) CorrectablePerNodeArg(ctx gorums.ServerCtx, request *Request) (response *Response, err error) {
	panic("CorrectablePerNodeArg not implemented")
}
func (srv *Server) CorrectableCustomReturnType(ctx gorums.ServerCtx, request *Request) (response *Response, err error) {
	panic("CorrectableCustomReturnType not implemented")
}
func (srv *Server) CorrectableCombo(ctx gorums.ServerCtx, request *Request) (response *Response, err error) {
	panic("CorrectableCombo not implemented")
}
func (srv *Server) CorrectableEmpty(ctx gorums.ServerCtx, request *Request) (response *empty.Empty, err error) {
	panic("CorrectableEmpty not implemented")
}
func (srv *Server) CorrectableEmpty2(ctx gorums.ServerCtx, request *empty.Empty) (response *Response, err error) {
	panic("CorrectableEmpty2 not implemented")
}
func (srv *Server) CorrectableStream(ctx gorums.ServerCtx, request *Request, send func(response *Response) error) error {
	panic("CorrectableStream not implemented")
}
func (srv *Server) CorrectableStreamPerNodeArg(ctx gorums.ServerCtx, request *Request, send func(response *Response) error) error {
	panic("CorrectableStreamPerNodeArg not implemented")
}
func (srv *Server) CorrectableStreamCustomReturnType(ctx gorums.ServerCtx, request *Request, send func(response *Response) error) error {
	panic("CorrectableStreamCustomReturnType not implemented")
}
func (srv *Server) CorrectableStreamCombo(ctx gorums.ServerCtx, request *Request, send func(response *Response) error) error {
	panic("CorrectableStreamCombo not implemented")
}
func (srv *Server) CorrectableStreamEmpty(ctx gorums.ServerCtx, request *Request, send func(response *empty.Empty) error) error {
	panic("CorrectableStreamEmpty not implemented")
}
func (srv *Server) CorrectableStreamEmpty2(ctx gorums.ServerCtx, request *empty.Empty, send func(response *Response) error) error {
	panic("CorrectableStreamEmpty2 not implemented")
}
func (srv *Server) Unicast(ctx gorums.ServerCtx, request *Request) {
	panic("Unicast not implemented")
}
func (srv *Server) Unicast2(ctx gorums.ServerCtx, request *Request) {
	panic("Unicast2 not implemented")
}

func RegisterZorumsServiceServer(srv *Server, impl ZorumsService) {
	srv.RegisterHandler("dev.ZorumsService.GRPCCall", func(ctx gorums.ServerCtx, in *gorums.Message, finished chan<- *gorums.Message) {
		req := in.Message.(*Request)
		defer ctx.Release()
		resp, err := impl.GRPCCall(ctx, req)
		gorums.SendMessage(ctx, finished, gorums.WrapMessage(in.Metadata, resp, err))
	})
	srv.RegisterHandler("dev.ZorumsService.QuorumCall", func(ctx gorums.ServerCtx, in *gorums.Message, finished chan<- *gorums.Message) {
		req := in.Message.(*Request)
		defer ctx.Release()
		resp, err := impl.QuorumCall(ctx, req)
		gorums.SendMessage(ctx, finished, gorums.WrapMessage(in.Metadata, resp, err))
	})
	srv.RegisterHandler("dev.ZorumsService.QuorumCallPerNodeArg", func(ctx gorums.ServerCtx, in *gorums.Message, finished chan<- *gorums.Message) {
		req := in.Message.(*Request)
		defer ctx.Release()
		resp, err := impl.QuorumCallPerNodeArg(ctx, req)
		gorums.SendMessage(ctx, finished, gorums.WrapMessage(in.Metadata, resp, err))
	})
	srv.RegisterHandler("dev.ZorumsService.QuorumCallCustomReturnType", func(ctx gorums.ServerCtx, in *gorums.Message, finished chan<- *gorums.Message) {
		req := in.Message.(*Request)
		defer ctx.Release()
		resp, err := impl.QuorumCallCustomReturnType(ctx, req)
		gorums.SendMessage(ctx, finished, gorums.WrapMessage(in.Metadata, resp, err))
	})
	srv.RegisterHandler("dev.ZorumsService.QuorumCallCombo", func(ctx gorums.ServerCtx, in *gorums.Message, finished chan<- *gorums.Message) {
		req := in.Message.(*Request)
		defer ctx.Release()
		resp, err := impl.QuorumCallCombo(ctx, req)
		gorums.SendMessage(ctx, finished, gorums.WrapMessage(in.Metadata, resp, err))
	})
	srv.RegisterHandler("dev.ZorumsService.QuorumCallEmpty", func(ctx gorums.ServerCtx, in *gorums.Message, finished chan<- *gorums.Message) {
		req := in.Message.(*empty.Empty)
		defer ctx.Release()
		resp, err := impl.QuorumCallEmpty(ctx, req)
		gorums.SendMessage(ctx, finished, gorums.WrapMessage(in.Metadata, resp, err))
	})
	srv.RegisterHandler("dev.ZorumsService.QuorumCallEmpty2", func(ctx gorums.ServerCtx, in *gorums.Message, finished chan<- *gorums.Message) {
		req := in.Message.(*Request)
		defer ctx.Release()
		resp, err := impl.QuorumCallEmpty2(ctx, req)
		gorums.SendMessage(ctx, finished, gorums.WrapMessage(in.Metadata, resp, err))
	})
	srv.RegisterHandler("dev.ZorumsService.Multiparty", gorums.BroadcastHandler(impl.Multiparty, srv.Server))
	srv.RegisterHandler("dev.ZorumsService.MultipartyInternal", gorums.BroadcastHandler(impl.MultipartyInternal, srv.Server))
	srv.RegisterHandler("dev.ZorumsService.Multicast", func(ctx gorums.ServerCtx, in *gorums.Message, _ chan<- *gorums.Message) {
		req := in.Message.(*Request)
		defer ctx.Release()
		impl.Multicast(ctx, req)
	})
	srv.RegisterHandler("dev.ZorumsService.MulticastPerNodeArg", func(ctx gorums.ServerCtx, in *gorums.Message, _ chan<- *gorums.Message) {
		req := in.Message.(*Request)
		defer ctx.Release()
		impl.MulticastPerNodeArg(ctx, req)
	})
	srv.RegisterHandler("dev.ZorumsService.Multicast2", func(ctx gorums.ServerCtx, in *gorums.Message, _ chan<- *gorums.Message) {
		req := in.Message.(*Request)
		defer ctx.Release()
		impl.Multicast2(ctx, req)
	})
	srv.RegisterHandler("dev.ZorumsService.Multicast3", func(ctx gorums.ServerCtx, in *gorums.Message, _ chan<- *gorums.Message) {
		req := in.Message.(*Request)
		defer ctx.Release()
		impl.Multicast3(ctx, req)
	})
	srv.RegisterHandler("dev.ZorumsService.Multicast4", func(ctx gorums.ServerCtx, in *gorums.Message, _ chan<- *gorums.Message) {
		req := in.Message.(*empty.Empty)
		defer ctx.Release()
		impl.Multicast4(ctx, req)
	})
	srv.RegisterHandler("dev.ZorumsService.QuorumCallAsync", func(ctx gorums.ServerCtx, in *gorums.Message, finished chan<- *gorums.Message) {
		req := in.Message.(*Request)
		defer ctx.Release()
		resp, err := impl.QuorumCallAsync(ctx, req)
		gorums.SendMessage(ctx, finished, gorums.WrapMessage(in.Metadata, resp, err))
	})
	srv.RegisterHandler("dev.ZorumsService.QuorumCallAsyncPerNodeArg", func(ctx gorums.ServerCtx, in *gorums.Message, finished chan<- *gorums.Message) {
		req := in.Message.(*Request)
		defer ctx.Release()
		resp, err := impl.QuorumCallAsyncPerNodeArg(ctx, req)
		gorums.SendMessage(ctx, finished, gorums.WrapMessage(in.Metadata, resp, err))
	})
	srv.RegisterHandler("dev.ZorumsService.QuorumCallAsyncCustomReturnType", func(ctx gorums.ServerCtx, in *gorums.Message, finished chan<- *gorums.Message) {
		req := in.Message.(*Request)
		defer ctx.Release()
		resp, err := impl.QuorumCallAsyncCustomReturnType(ctx, req)
		gorums.SendMessage(ctx, finished, gorums.WrapMessage(in.Metadata, resp, err))
	})
	srv.RegisterHandler("dev.ZorumsService.QuorumCallAsyncCombo", func(ctx gorums.ServerCtx, in *gorums.Message, finished chan<- *gorums.Message) {
		req := in.Message.(*Request)
		defer ctx.Release()
		resp, err := impl.QuorumCallAsyncCombo(ctx, req)
		gorums.SendMessage(ctx, finished, gorums.WrapMessage(in.Metadata, resp, err))
	})
	srv.RegisterHandler("dev.ZorumsService.QuorumCallAsync2", func(ctx gorums.ServerCtx, in *gorums.Message, finished chan<- *gorums.Message) {
		req := in.Message.(*Request)
		defer ctx.Release()
		resp, err := impl.QuorumCallAsync2(ctx, req)
		gorums.SendMessage(ctx, finished, gorums.WrapMessage(in.Metadata, resp, err))
	})
	srv.RegisterHandler("dev.ZorumsService.QuorumCallAsyncEmpty", func(ctx gorums.ServerCtx, in *gorums.Message, finished chan<- *gorums.Message) {
		req := in.Message.(*Request)
		defer ctx.Release()
		resp, err := impl.QuorumCallAsyncEmpty(ctx, req)
		gorums.SendMessage(ctx, finished, gorums.WrapMessage(in.Metadata, resp, err))
	})
	srv.RegisterHandler("dev.ZorumsService.QuorumCallAsyncEmpty2", func(ctx gorums.ServerCtx, in *gorums.Message, finished chan<- *gorums.Message) {
		req := in.Message.(*empty.Empty)
		defer ctx.Release()
		resp, err := impl.QuorumCallAsyncEmpty2(ctx, req)
		gorums.SendMessage(ctx, finished, gorums.WrapMessage(in.Metadata, resp, err))
	})
	srv.RegisterHandler("dev.ZorumsService.Correctable", func(ctx gorums.ServerCtx, in *gorums.Message, finished chan<- *gorums.Message) {
		req := in.Message.(*Request)
		defer ctx.Release()
		resp, err := impl.Correctable(ctx, req)
		gorums.SendMessage(ctx, finished, gorums.WrapMessage(in.Metadata, resp, err))
	})
	srv.RegisterHandler("dev.ZorumsService.CorrectablePerNodeArg", func(ctx gorums.ServerCtx, in *gorums.Message, finished chan<- *gorums.Message) {
		req := in.Message.(*Request)
		defer ctx.Release()
		resp, err := impl.CorrectablePerNodeArg(ctx, req)
		gorums.SendMessage(ctx, finished, gorums.WrapMessage(in.Metadata, resp, err))
	})
	srv.RegisterHandler("dev.ZorumsService.CorrectableCustomReturnType", func(ctx gorums.ServerCtx, in *gorums.Message, finished chan<- *gorums.Message) {
		req := in.Message.(*Request)
		defer ctx.Release()
		resp, err := impl.CorrectableCustomReturnType(ctx, req)
		gorums.SendMessage(ctx, finished, gorums.WrapMessage(in.Metadata, resp, err))
	})
	srv.RegisterHandler("dev.ZorumsService.CorrectableCombo", func(ctx gorums.ServerCtx, in *gorums.Message, finished chan<- *gorums.Message) {
		req := in.Message.(*Request)
		defer ctx.Release()
		resp, err := impl.CorrectableCombo(ctx, req)
		gorums.SendMessage(ctx, finished, gorums.WrapMessage(in.Metadata, resp, err))
	})
	srv.RegisterHandler("dev.ZorumsService.CorrectableEmpty", func(ctx gorums.ServerCtx, in *gorums.Message, finished chan<- *gorums.Message) {
		req := in.Message.(*Request)
		defer ctx.Release()
		resp, err := impl.CorrectableEmpty(ctx, req)
		gorums.SendMessage(ctx, finished, gorums.WrapMessage(in.Metadata, resp, err))
	})
	srv.RegisterHandler("dev.ZorumsService.CorrectableEmpty2", func(ctx gorums.ServerCtx, in *gorums.Message, finished chan<- *gorums.Message) {
		req := in.Message.(*empty.Empty)
		defer ctx.Release()
		resp, err := impl.CorrectableEmpty2(ctx, req)
		gorums.SendMessage(ctx, finished, gorums.WrapMessage(in.Metadata, resp, err))
	})
	srv.RegisterHandler("dev.ZorumsService.CorrectableStream", func(ctx gorums.ServerCtx, in *gorums.Message, finished chan<- *gorums.Message) {
		req := in.Message.(*Request)
		defer ctx.Release()
		err := impl.CorrectableStream(ctx, req, func(resp *Response) error {
			// create a copy of the metadata, to avoid a data race between WrapMessage and SendMsg
			md := proto.Clone(in.Metadata)
			return gorums.SendMessage(ctx, finished, gorums.WrapMessage(md.(*ordering.Metadata), resp, nil))
		})
		if err != nil {
			gorums.SendMessage(ctx, finished, gorums.WrapMessage(in.Metadata, nil, err))
		}
	})
	srv.RegisterHandler("dev.ZorumsService.CorrectableStreamPerNodeArg", func(ctx gorums.ServerCtx, in *gorums.Message, finished chan<- *gorums.Message) {
		req := in.Message.(*Request)
		defer ctx.Release()
		err := impl.CorrectableStreamPerNodeArg(ctx, req, func(resp *Response) error {
			// create a copy of the metadata, to avoid a data race between WrapMessage and SendMsg
			md := proto.Clone(in.Metadata)
			return gorums.SendMessage(ctx, finished, gorums.WrapMessage(md.(*ordering.Metadata), resp, nil))
		})
		if err != nil {
			gorums.SendMessage(ctx, finished, gorums.WrapMessage(in.Metadata, nil, err))
		}
	})
	srv.RegisterHandler("dev.ZorumsService.CorrectableStreamCustomReturnType", func(ctx gorums.ServerCtx, in *gorums.Message, finished chan<- *gorums.Message) {
		req := in.Message.(*Request)
		defer ctx.Release()
		err := impl.CorrectableStreamCustomReturnType(ctx, req, func(resp *Response) error {
			// create a copy of the metadata, to avoid a data race between WrapMessage and SendMsg
			md := proto.Clone(in.Metadata)
			return gorums.SendMessage(ctx, finished, gorums.WrapMessage(md.(*ordering.Metadata), resp, nil))
		})
		if err != nil {
			gorums.SendMessage(ctx, finished, gorums.WrapMessage(in.Metadata, nil, err))
		}
	})
	srv.RegisterHandler("dev.ZorumsService.CorrectableStreamCombo", func(ctx gorums.ServerCtx, in *gorums.Message, finished chan<- *gorums.Message) {
		req := in.Message.(*Request)
		defer ctx.Release()
		err := impl.CorrectableStreamCombo(ctx, req, func(resp *Response) error {
			// create a copy of the metadata, to avoid a data race between WrapMessage and SendMsg
			md := proto.Clone(in.Metadata)
			return gorums.SendMessage(ctx, finished, gorums.WrapMessage(md.(*ordering.Metadata), resp, nil))
		})
		if err != nil {
			gorums.SendMessage(ctx, finished, gorums.WrapMessage(in.Metadata, nil, err))
		}
	})
	srv.RegisterHandler("dev.ZorumsService.CorrectableStreamEmpty", func(ctx gorums.ServerCtx, in *gorums.Message, finished chan<- *gorums.Message) {
		req := in.Message.(*Request)
		defer ctx.Release()
		err := impl.CorrectableStreamEmpty(ctx, req, func(resp *empty.Empty) error {
			// create a copy of the metadata, to avoid a data race between WrapMessage and SendMsg
			md := proto.Clone(in.Metadata)
			return gorums.SendMessage(ctx, finished, gorums.WrapMessage(md.(*ordering.Metadata), resp, nil))
		})
		if err != nil {
			gorums.SendMessage(ctx, finished, gorums.WrapMessage(in.Metadata, nil, err))
		}
	})
	srv.RegisterHandler("dev.ZorumsService.CorrectableStreamEmpty2", func(ctx gorums.ServerCtx, in *gorums.Message, finished chan<- *gorums.Message) {
		req := in.Message.(*empty.Empty)
		defer ctx.Release()
		err := impl.CorrectableStreamEmpty2(ctx, req, func(resp *Response) error {
			// create a copy of the metadata, to avoid a data race between WrapMessage and SendMsg
			md := proto.Clone(in.Metadata)
			return gorums.SendMessage(ctx, finished, gorums.WrapMessage(md.(*ordering.Metadata), resp, nil))
		})
		if err != nil {
			gorums.SendMessage(ctx, finished, gorums.WrapMessage(in.Metadata, nil, err))
		}
	})
	srv.RegisterHandler("dev.ZorumsService.Unicast", func(ctx gorums.ServerCtx, in *gorums.Message, _ chan<- *gorums.Message) {
		req := in.Message.(*Request)
		defer ctx.Release()
		impl.Unicast(ctx, req)
	})
	srv.RegisterHandler("dev.ZorumsService.Unicast2", func(ctx gorums.ServerCtx, in *gorums.Message, _ chan<- *gorums.Message) {
		req := in.Message.(*Request)
		defer ctx.Release()
		impl.Unicast2(ctx, req)
	})
}

func (b *Broadcast) ReturnToClient(resp *ClientResponse, err error) {
	b.sp.ReturnToClientHandler(resp, err, b.metadata)
}

func (srv *Server) ReturnToClient(resp *ClientResponse, err error, broadcastID string) {
	srv.RetToClient(resp, err, broadcastID)
}
