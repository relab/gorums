// Code generated by protoc-gen-gorums. DO NOT EDIT.
// versions:
// 	protoc-gen-gorums v0.5.0-devel
// 	protoc            v3.15.8
// source: zorums.proto

package dev

import (
	context "context"
	gorums "github.com/relab/gorums"
	emptypb "google.golang.org/protobuf/types/known/emptypb"
	sync "sync"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = gorums.EnforceVersion(5 - gorums.MinVersion)
	// Verify that the gorums runtime is sufficiently up-to-date.
	_ = gorums.EnforceVersion(gorums.MaxVersion - 5)
)

// ZorumsService is the server-side API for the ZorumsService Service
type ZorumsService interface {
	GRPCCall(context.Context, *Request, func(*Response, error))
	QuorumCall(context.Context, *Request, func(*Response, error))
	QuorumCallPerNodeArg(context.Context, *Request, func(*Response, error))
	QuorumCallCustomReturnType(context.Context, *Request, func(*Response, error))
	QuorumCallCombo(context.Context, *Request, func(*Response, error))
	QuorumCallEmpty(context.Context, *emptypb.Empty, func(*Response, error))
	QuorumCallEmpty2(context.Context, *Request, func(*emptypb.Empty, error))
	Multicast(context.Context, *Request)
	MulticastPerNodeArg(context.Context, *Request)
	Multicast2(context.Context, *Request)
	Multicast3(context.Context, *Request)
	Multicast4(context.Context, *emptypb.Empty)
	QuorumCallAsync(context.Context, *Request, func(*Response, error))
	QuorumCallAsyncPerNodeArg(context.Context, *Request, func(*Response, error))
	QuorumCallAsyncCustomReturnType(context.Context, *Request, func(*Response, error))
	QuorumCallAsyncCombo(context.Context, *Request, func(*Response, error))
	QuorumCallAsync2(context.Context, *Request, func(*Response, error))
	QuorumCallAsyncEmpty(context.Context, *Request, func(*emptypb.Empty, error))
	QuorumCallAsyncEmpty2(context.Context, *emptypb.Empty, func(*Response, error))
	Correctable(context.Context, *Request, func(*Response, error))
	CorrectablePerNodeArg(context.Context, *Request, func(*Response, error))
	CorrectableCustomReturnType(context.Context, *Request, func(*Response, error))
	CorrectableCombo(context.Context, *Request, func(*Response, error))
	CorrectableEmpty(context.Context, *Request, func(*emptypb.Empty, error))
	CorrectableEmpty2(context.Context, *emptypb.Empty, func(*Response, error))
	CorrectableStream(context.Context, *Request, func(*Response, error))
	CorrectableStreamPerNodeArg(context.Context, *Request, func(*Response, error))
	CorrectableStreamCustomReturnType(context.Context, *Request, func(*Response, error))
	CorrectableStreamCombo(context.Context, *Request, func(*Response, error))
	CorrectableStreamEmpty(context.Context, *Request, func(*emptypb.Empty, error))
	CorrectableStreamEmpty2(context.Context, *emptypb.Empty, func(*Response, error))
	Unicast(context.Context, *Request)
	Unicast2(context.Context, *Request)
}

func RegisterZorumsServiceServer(srv *gorums.Server, impl ZorumsService) {
	srv.RegisterHandler("dev.ZorumsService.GRPCCall", func(ctx context.Context, in *gorums.Message, finished chan<- *gorums.Message) {
		req := in.Message.(*Request)
		once := new(sync.Once)
		f := func(resp *Response, err error) {
			once.Do(func() {
				select {
				case finished <- gorums.WrapMessage(in.Metadata, resp, err):
				case <-ctx.Done():
				}
			})
		}
		impl.GRPCCall(ctx, req, f)
	})
	srv.RegisterHandler("dev.ZorumsService.QuorumCall", func(ctx context.Context, in *gorums.Message, finished chan<- *gorums.Message) {
		req := in.Message.(*Request)
		once := new(sync.Once)
		f := func(resp *Response, err error) {
			once.Do(func() {
				select {
				case finished <- gorums.WrapMessage(in.Metadata, resp, err):
				case <-ctx.Done():
				}
			})
		}
		impl.QuorumCall(ctx, req, f)
	})
	srv.RegisterHandler("dev.ZorumsService.QuorumCallPerNodeArg", func(ctx context.Context, in *gorums.Message, finished chan<- *gorums.Message) {
		req := in.Message.(*Request)
		once := new(sync.Once)
		f := func(resp *Response, err error) {
			once.Do(func() {
				select {
				case finished <- gorums.WrapMessage(in.Metadata, resp, err):
				case <-ctx.Done():
				}
			})
		}
		impl.QuorumCallPerNodeArg(ctx, req, f)
	})
	srv.RegisterHandler("dev.ZorumsService.QuorumCallCustomReturnType", func(ctx context.Context, in *gorums.Message, finished chan<- *gorums.Message) {
		req := in.Message.(*Request)
		once := new(sync.Once)
		f := func(resp *Response, err error) {
			once.Do(func() {
				select {
				case finished <- gorums.WrapMessage(in.Metadata, resp, err):
				case <-ctx.Done():
				}
			})
		}
		impl.QuorumCallCustomReturnType(ctx, req, f)
	})
	srv.RegisterHandler("dev.ZorumsService.QuorumCallCombo", func(ctx context.Context, in *gorums.Message, finished chan<- *gorums.Message) {
		req := in.Message.(*Request)
		once := new(sync.Once)
		f := func(resp *Response, err error) {
			once.Do(func() {
				select {
				case finished <- gorums.WrapMessage(in.Metadata, resp, err):
				case <-ctx.Done():
				}
			})
		}
		impl.QuorumCallCombo(ctx, req, f)
	})
	srv.RegisterHandler("dev.ZorumsService.QuorumCallEmpty", func(ctx context.Context, in *gorums.Message, finished chan<- *gorums.Message) {
		req := in.Message.(*emptypb.Empty)
		once := new(sync.Once)
		f := func(resp *Response, err error) {
			once.Do(func() {
				select {
				case finished <- gorums.WrapMessage(in.Metadata, resp, err):
				case <-ctx.Done():
				}
			})
		}
		impl.QuorumCallEmpty(ctx, req, f)
	})
	srv.RegisterHandler("dev.ZorumsService.QuorumCallEmpty2", func(ctx context.Context, in *gorums.Message, finished chan<- *gorums.Message) {
		req := in.Message.(*Request)
		once := new(sync.Once)
		f := func(resp *emptypb.Empty, err error) {
			once.Do(func() {
				select {
				case finished <- gorums.WrapMessage(in.Metadata, resp, err):
				case <-ctx.Done():
				}
			})
		}
		impl.QuorumCallEmpty2(ctx, req, f)
	})
	srv.RegisterHandler("dev.ZorumsService.Multicast", func(ctx context.Context, in *gorums.Message, _ chan<- *gorums.Message) {
		req := in.Message.(*Request)
		impl.Multicast(ctx, req)
	})
	srv.RegisterHandler("dev.ZorumsService.MulticastPerNodeArg", func(ctx context.Context, in *gorums.Message, _ chan<- *gorums.Message) {
		req := in.Message.(*Request)
		impl.MulticastPerNodeArg(ctx, req)
	})
	srv.RegisterHandler("dev.ZorumsService.Multicast2", func(ctx context.Context, in *gorums.Message, _ chan<- *gorums.Message) {
		req := in.Message.(*Request)
		impl.Multicast2(ctx, req)
	})
	srv.RegisterHandler("dev.ZorumsService.Multicast3", func(ctx context.Context, in *gorums.Message, _ chan<- *gorums.Message) {
		req := in.Message.(*Request)
		impl.Multicast3(ctx, req)
	})
	srv.RegisterHandler("dev.ZorumsService.Multicast4", func(ctx context.Context, in *gorums.Message, _ chan<- *gorums.Message) {
		req := in.Message.(*emptypb.Empty)
		impl.Multicast4(ctx, req)
	})
	srv.RegisterHandler("dev.ZorumsService.QuorumCallAsync", func(ctx context.Context, in *gorums.Message, finished chan<- *gorums.Message) {
		req := in.Message.(*Request)
		once := new(sync.Once)
		f := func(resp *Response, err error) {
			once.Do(func() {
				select {
				case finished <- gorums.WrapMessage(in.Metadata, resp, err):
				case <-ctx.Done():
				}
			})
		}
		impl.QuorumCallAsync(ctx, req, f)
	})
	srv.RegisterHandler("dev.ZorumsService.QuorumCallAsyncPerNodeArg", func(ctx context.Context, in *gorums.Message, finished chan<- *gorums.Message) {
		req := in.Message.(*Request)
		once := new(sync.Once)
		f := func(resp *Response, err error) {
			once.Do(func() {
				select {
				case finished <- gorums.WrapMessage(in.Metadata, resp, err):
				case <-ctx.Done():
				}
			})
		}
		impl.QuorumCallAsyncPerNodeArg(ctx, req, f)
	})
	srv.RegisterHandler("dev.ZorumsService.QuorumCallAsyncCustomReturnType", func(ctx context.Context, in *gorums.Message, finished chan<- *gorums.Message) {
		req := in.Message.(*Request)
		once := new(sync.Once)
		f := func(resp *Response, err error) {
			once.Do(func() {
				select {
				case finished <- gorums.WrapMessage(in.Metadata, resp, err):
				case <-ctx.Done():
				}
			})
		}
		impl.QuorumCallAsyncCustomReturnType(ctx, req, f)
	})
	srv.RegisterHandler("dev.ZorumsService.QuorumCallAsyncCombo", func(ctx context.Context, in *gorums.Message, finished chan<- *gorums.Message) {
		req := in.Message.(*Request)
		once := new(sync.Once)
		f := func(resp *Response, err error) {
			once.Do(func() {
				select {
				case finished <- gorums.WrapMessage(in.Metadata, resp, err):
				case <-ctx.Done():
				}
			})
		}
		impl.QuorumCallAsyncCombo(ctx, req, f)
	})
	srv.RegisterHandler("dev.ZorumsService.QuorumCallAsync2", func(ctx context.Context, in *gorums.Message, finished chan<- *gorums.Message) {
		req := in.Message.(*Request)
		once := new(sync.Once)
		f := func(resp *Response, err error) {
			once.Do(func() {
				select {
				case finished <- gorums.WrapMessage(in.Metadata, resp, err):
				case <-ctx.Done():
				}
			})
		}
		impl.QuorumCallAsync2(ctx, req, f)
	})
	srv.RegisterHandler("dev.ZorumsService.QuorumCallAsyncEmpty", func(ctx context.Context, in *gorums.Message, finished chan<- *gorums.Message) {
		req := in.Message.(*Request)
		once := new(sync.Once)
		f := func(resp *emptypb.Empty, err error) {
			once.Do(func() {
				select {
				case finished <- gorums.WrapMessage(in.Metadata, resp, err):
				case <-ctx.Done():
				}
			})
		}
		impl.QuorumCallAsyncEmpty(ctx, req, f)
	})
	srv.RegisterHandler("dev.ZorumsService.QuorumCallAsyncEmpty2", func(ctx context.Context, in *gorums.Message, finished chan<- *gorums.Message) {
		req := in.Message.(*emptypb.Empty)
		once := new(sync.Once)
		f := func(resp *Response, err error) {
			once.Do(func() {
				select {
				case finished <- gorums.WrapMessage(in.Metadata, resp, err):
				case <-ctx.Done():
				}
			})
		}
		impl.QuorumCallAsyncEmpty2(ctx, req, f)
	})
	srv.RegisterHandler("dev.ZorumsService.Correctable", func(ctx context.Context, in *gorums.Message, finished chan<- *gorums.Message) {
		req := in.Message.(*Request)
		once := new(sync.Once)
		f := func(resp *Response, err error) {
			once.Do(func() {
				select {
				case finished <- gorums.WrapMessage(in.Metadata, resp, err):
				case <-ctx.Done():
				}
			})
		}
		impl.Correctable(ctx, req, f)
	})
	srv.RegisterHandler("dev.ZorumsService.CorrectablePerNodeArg", func(ctx context.Context, in *gorums.Message, finished chan<- *gorums.Message) {
		req := in.Message.(*Request)
		once := new(sync.Once)
		f := func(resp *Response, err error) {
			once.Do(func() {
				select {
				case finished <- gorums.WrapMessage(in.Metadata, resp, err):
				case <-ctx.Done():
				}
			})
		}
		impl.CorrectablePerNodeArg(ctx, req, f)
	})
	srv.RegisterHandler("dev.ZorumsService.CorrectableCustomReturnType", func(ctx context.Context, in *gorums.Message, finished chan<- *gorums.Message) {
		req := in.Message.(*Request)
		once := new(sync.Once)
		f := func(resp *Response, err error) {
			once.Do(func() {
				select {
				case finished <- gorums.WrapMessage(in.Metadata, resp, err):
				case <-ctx.Done():
				}
			})
		}
		impl.CorrectableCustomReturnType(ctx, req, f)
	})
	srv.RegisterHandler("dev.ZorumsService.CorrectableCombo", func(ctx context.Context, in *gorums.Message, finished chan<- *gorums.Message) {
		req := in.Message.(*Request)
		once := new(sync.Once)
		f := func(resp *Response, err error) {
			once.Do(func() {
				select {
				case finished <- gorums.WrapMessage(in.Metadata, resp, err):
				case <-ctx.Done():
				}
			})
		}
		impl.CorrectableCombo(ctx, req, f)
	})
	srv.RegisterHandler("dev.ZorumsService.CorrectableEmpty", func(ctx context.Context, in *gorums.Message, finished chan<- *gorums.Message) {
		req := in.Message.(*Request)
		once := new(sync.Once)
		f := func(resp *emptypb.Empty, err error) {
			once.Do(func() {
				select {
				case finished <- gorums.WrapMessage(in.Metadata, resp, err):
				case <-ctx.Done():
				}
			})
		}
		impl.CorrectableEmpty(ctx, req, f)
	})
	srv.RegisterHandler("dev.ZorumsService.CorrectableEmpty2", func(ctx context.Context, in *gorums.Message, finished chan<- *gorums.Message) {
		req := in.Message.(*emptypb.Empty)
		once := new(sync.Once)
		f := func(resp *Response, err error) {
			once.Do(func() {
				select {
				case finished <- gorums.WrapMessage(in.Metadata, resp, err):
				case <-ctx.Done():
				}
			})
		}
		impl.CorrectableEmpty2(ctx, req, f)
	})
	srv.RegisterHandler("dev.ZorumsService.CorrectableStream", func(ctx context.Context, in *gorums.Message, finished chan<- *gorums.Message) {
		req := in.Message.(*Request)
		once := new(sync.Once)
		f := func(resp *Response, err error) {
			once.Do(func() {
				select {
				case finished <- gorums.WrapMessage(in.Metadata, resp, err):
				case <-ctx.Done():
				}
			})
		}
		impl.CorrectableStream(ctx, req, f)
	})
	srv.RegisterHandler("dev.ZorumsService.CorrectableStreamPerNodeArg", func(ctx context.Context, in *gorums.Message, finished chan<- *gorums.Message) {
		req := in.Message.(*Request)
		once := new(sync.Once)
		f := func(resp *Response, err error) {
			once.Do(func() {
				select {
				case finished <- gorums.WrapMessage(in.Metadata, resp, err):
				case <-ctx.Done():
				}
			})
		}
		impl.CorrectableStreamPerNodeArg(ctx, req, f)
	})
	srv.RegisterHandler("dev.ZorumsService.CorrectableStreamCustomReturnType", func(ctx context.Context, in *gorums.Message, finished chan<- *gorums.Message) {
		req := in.Message.(*Request)
		once := new(sync.Once)
		f := func(resp *Response, err error) {
			once.Do(func() {
				select {
				case finished <- gorums.WrapMessage(in.Metadata, resp, err):
				case <-ctx.Done():
				}
			})
		}
		impl.CorrectableStreamCustomReturnType(ctx, req, f)
	})
	srv.RegisterHandler("dev.ZorumsService.CorrectableStreamCombo", func(ctx context.Context, in *gorums.Message, finished chan<- *gorums.Message) {
		req := in.Message.(*Request)
		once := new(sync.Once)
		f := func(resp *Response, err error) {
			once.Do(func() {
				select {
				case finished <- gorums.WrapMessage(in.Metadata, resp, err):
				case <-ctx.Done():
				}
			})
		}
		impl.CorrectableStreamCombo(ctx, req, f)
	})
	srv.RegisterHandler("dev.ZorumsService.CorrectableStreamEmpty", func(ctx context.Context, in *gorums.Message, finished chan<- *gorums.Message) {
		req := in.Message.(*Request)
		once := new(sync.Once)
		f := func(resp *emptypb.Empty, err error) {
			once.Do(func() {
				select {
				case finished <- gorums.WrapMessage(in.Metadata, resp, err):
				case <-ctx.Done():
				}
			})
		}
		impl.CorrectableStreamEmpty(ctx, req, f)
	})
	srv.RegisterHandler("dev.ZorumsService.CorrectableStreamEmpty2", func(ctx context.Context, in *gorums.Message, finished chan<- *gorums.Message) {
		req := in.Message.(*emptypb.Empty)
		once := new(sync.Once)
		f := func(resp *Response, err error) {
			once.Do(func() {
				select {
				case finished <- gorums.WrapMessage(in.Metadata, resp, err):
				case <-ctx.Done():
				}
			})
		}
		impl.CorrectableStreamEmpty2(ctx, req, f)
	})
	srv.RegisterHandler("dev.ZorumsService.Unicast", func(ctx context.Context, in *gorums.Message, _ chan<- *gorums.Message) {
		req := in.Message.(*Request)
		impl.Unicast(ctx, req)
	})
	srv.RegisterHandler("dev.ZorumsService.Unicast2", func(ctx context.Context, in *gorums.Message, _ chan<- *gorums.Message) {
		req := in.Message.(*Request)
		impl.Unicast2(ctx, req)
	})
}
