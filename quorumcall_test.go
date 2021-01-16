package gorums_test

// func BenchmarkQuorumCall(b *testing.B) {
// 	cd := gorums.QuorumCallData{
// 		Manager: c.mgr.Manager,
// 		Nodes:   c.nodes,
// 		Message: in,
// 		Method:  "dev.ZorumsService.QuorumCall",
// 	}
// 	cd.QuorumFunction = func(req protoreflect.ProtoMessage, replies map[uint32]protoreflect.ProtoMessage) (protoreflect.ProtoMessage, bool) {
// 		// 	if len(replies) < qspec.CfgSize {
// 		// 		return nil, false
// 		// 	}
// 		// return any response
// 		var resp protoreflect.ProtoMessage
// 		for _, r := range replies {
// 			resp = r
// 			break
// 		}
// 		return resp, true
// 	}
// 	ctx := context.Background()
// 	_, err := gorums.QuorumCall(ctx, cd)
// 	if err != nil {
// 		b.Fatal(err)
// 	}
// }
