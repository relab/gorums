package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/relab/gorums"
	pb "github.com/relab/gorums/examples/storage/proto"
	"golang.org/x/term"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var help = `
This interface allows you to run RPCs and quorum calls against the Storage
Servers interactively. Take a look at the files 'client.go' and 'server.go'
for the source code of the RPC handlers and quorum functions.
The following commands can be used:

help                             Show this text
exit                             Exit the program
nodes                            Print a list of the available nodes
rpc   [node index] [operation]	 Executes an RPC on the given node.
qc    [operation]             	 Executes a quorum call on all nodes.
mcast [key] [value]              Executes a multicast write call on all nodes.
ucast [node index] [key] [value] Executes a unicast write call on one node.
cfg   [config] [operation]   	 Executes a quorum call on a configuration.

The following operations are supported:

read 	[key]        	Read a value
cread   [key]           Correctable Read a value (streams updates)
write	[key] [value]	Write a value
nread   [key]           Nested quorum call
nwrite  [key] [value]   Nested multicast

Examples:

> rpc 0 write foo bar
The command performs the 'write' RPC on node 0, and sets 'foo' = 'bar'

> qc read foo
The command performs the 'read' quorum call, and returns the value of 'foo'

> cfg 1:3 write foo bar
The command performs the write quorum call on node 1 and 2

> cfg 0,2 write foo 'bar baz'
The command performs the write quorum call on node 0 and 2
`

var delayOutput = 200 * time.Millisecond

type repl struct {
	mgr  *pb.Manager
	cfg  pb.Configuration
	term *term.Terminal
}

func newRepl(mgr *pb.Manager, cfg pb.Configuration) *repl {
	return &repl{
		mgr: mgr,
		cfg: cfg,
		term: term.NewTerminal(struct {
			io.Reader
			io.Writer
		}{os.Stdin, os.Stderr}, "> "),
	}
}

// ReadLine reads a line from the terminal in raw mode.
func (r repl) ReadLine() (string, error) {
	fd := int(os.Stdin.Fd())
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		panic(err)
	}
	defer func() {
		err := term.Restore(fd, oldState)
		if err != nil {
			panic(err)
		}
	}()

	return r.term.ReadLine()
}

// Repl runs an interactive Read-eval-print loop, that allows users to run commands that perform
// RPCs and quorum calls using the manager and configuration.
func Repl(mgr *pb.Manager, defaultCfg pb.Configuration) error {
	r := newRepl(mgr, defaultCfg)

	fmt.Println(help)
	for {
		l, err := r.ReadLine()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to read line: %v\n", err)
			return err
		}
		args, err := splitQuoted(l)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to split command arguments: %v\n", err)
			continue
		}
		if len(args) < 1 {
			continue
		}

		switch args[0] {
		case "exit":
			fallthrough
		case "quit":
			return nil
		case "help":
			fmt.Println(help)
		case "rpc":
			r.rpc(args[1:])
		case "qc":
			fallthrough
		case "quorumcall":
			r.qc(args[1:])
		case "cfg":
			r.qcCfg(args[1:])
		case "ucast":
			fallthrough
		case "unicast":
			r.unicast(args[1:])
		case "mcast":
			fallthrough
		case "multicast":
			r.multicast(args[1:])
		case "nodes":
			fmt.Println("Nodes: ")
			for i, n := range mgr.Nodes() {
				fmt.Printf("%d: %s\n", i, n.Address())
			}
		default:
			fmt.Printf("Unknown command '%s'. Type 'help' to see available commands.\n", args[0])
		}
	}
}

func (r repl) rpc(args []string) {
	if len(args) < 2 {
		fmt.Println("'rpc' requires a node index and an operation.")
		return
	}

	index, err := strconv.Atoi(args[0])
	if err != nil {
		fmt.Printf("Invalid id '%s'. node index must be numeric.\n", args[0])
		return
	}

	if index < 0 || index >= r.cfg.Size() {
		fmt.Printf("Invalid index. Must be between 0 and %d.\n", r.cfg.Size()-1)
		return
	}

	node := r.cfg[index]

	switch args[1] {
	case "read":
		r.readRPC(args[2:], node)
	case "write":
		r.writeRPC(args[2:], node)
	}
}

func (r repl) unicast(args []string) {
	if len(args) < 3 {
		fmt.Println("'unicast' requires a node index, a key, and a value.")
		return
	}

	index, err := strconv.Atoi(args[0])
	if err != nil {
		fmt.Printf("Invalid id '%s'. node index must be numeric.\n", args[0])
		return
	}

	if index < 0 || index >= r.cfg.Size() {
		fmt.Printf("Invalid index. Must be between 0 and %d.\n", r.cfg.Size()-1)
		return
	}

	node := r.cfg[index]
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	nodeCtx := node.Context(ctx)
	err = pb.WriteUnicast(nodeCtx, pb.WriteRequest_builder{
		Key: args[1], Value: args[2], Time: timestamppb.Now(),
	}.Build())
	cancel()
	if err != nil {
		fmt.Printf("Write unicast failed to send: %v\n", err)
		return
	}
	time.Sleep(delayOutput)
	fmt.Println("Unicast OK")
}

func (r repl) multicast(args []string) {
	if len(args) < 2 {
		fmt.Println("'multicast' requires a key and a value.")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	cfgCtx := r.cfg.Context(ctx)
	err := pb.WriteMulticast(cfgCtx, pb.WriteRequest_builder{
		Key: args[0], Value: args[1], Time: timestamppb.Now(),
	}.Build())
	cancel()
	if err != nil {
		fmt.Printf("Write multicast failed to send: %v\n", err)
		return
	}
	time.Sleep(delayOutput)
	fmt.Println("Multicast OK")
}

func (r repl) qc(args []string) {
	if len(args) < 1 {
		fmt.Println("'qc' requires an operation.")
		return
	}

	switch args[0] {
	case "read":
		r.readQC(args[1:], r.cfg)
	case "cread":
		r.creadQC(args[1:], r.cfg)
	case "write":
		r.writeQC(args[1:], r.cfg)
	case "nread":
		r.readNestedQC(args[1:], r.cfg)
	case "nwrite":
		r.writeNestedMulticast(args[1:], r.cfg)
	}
}

func (r repl) qcCfg(args []string) {
	if len(args) < 2 {
		fmt.Println("'cfg' requires a configuration and an operation.")
		return
	}
	cfg, err := r.parseConfiguration(args[0])
	if err != nil {
		fmt.Printf("Failed to parse configuration: %v\n", err)
		return
	}
	switch args[1] {
	case "read":
		r.readQC(args[2:], cfg)
	case "cread":
		r.creadQC(args[2:], cfg)
	case "write":
		r.writeQC(args[2:], cfg)
	case "nread":
		r.readNestedQC(args[2:], cfg)
	case "nwrite":
		r.writeNestedMulticast(args[2:], cfg)
	}
}

func (repl) readRPC(args []string, node *pb.Node) {
	if len(args) < 1 {
		fmt.Println("Read requires a key to read.")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	nodeCtx := node.Context(ctx)
	resp, err := pb.ReadRPC(nodeCtx, pb.ReadRequest_builder{Key: args[0]}.Build())
	cancel()
	if err != nil {
		fmt.Printf("Read RPC finished with error: %v\n", err)
		return
	}
	if !resp.GetOK() {
		fmt.Printf("%s was not found\n", args[0])
		return
	}
	fmt.Printf("%s = %s\n", args[0], resp.GetValue())
}

func (repl) writeRPC(args []string, node *pb.Node) {
	if len(args) < 2 {
		fmt.Println("Write requires a key and a value to write.")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	nodeCtx := node.Context(ctx)
	resp, err := pb.WriteRPC(nodeCtx, pb.WriteRequest_builder{
		Key: args[0], Value: args[1], Time: timestamppb.Now(),
	}.Build())
	cancel()
	if err != nil {
		fmt.Printf("Write RPC finished with error: %v\n", err)
		return
	}
	if !resp.GetNew() {
		fmt.Printf("Failed to update %s: timestamp too old.\n", args[0])
		return
	}
	fmt.Println("Write OK")
}

func (repl) readQC(args []string, config pb.Configuration) {
	if len(args) < 1 {
		fmt.Println("Read requires a key to read.")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	cfgCtx := config.Context(ctx)
	// Use the responses iterator to find the newest value
	resp, err := newestValue(pb.ReadQC(cfgCtx, pb.ReadRequest_builder{Key: args[0]}.Build()))
	cancel()
	if err != nil {
		fmt.Printf("Read RPC finished with error: %v\n", err)
		return
	}
	if !resp.GetOK() {
		fmt.Printf("%s was not found\n", args[0])
		return
	}
	fmt.Printf("%s = %s\n", args[0], resp.GetValue())
}

func (repl) creadQC(args []string, config pb.Configuration) {
	if len(args) < 1 {
		fmt.Println("Correctable Read requires a key to read.")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	cfgCtx := config.Context(ctx)

	// Create a correctable call expecting at least majority
	majority := (config.Size() + 1) / 2
	corr := pb.ReadCorrectable(cfgCtx, pb.ReadRequest_builder{Key: args[0]}.Build()).Correctable(majority)

	// Process updates as they arrive
	for level := 1; level <= majority; level++ {
		<-corr.Watch(level)
		resp, currentLevel, err := corr.Get()
		if err != nil {
			fmt.Printf("Read correctable error at level %d: %v\n", currentLevel, err)
			break
		}
		if !resp.GetOK() {
			fmt.Printf("%s was not found (level %d)\n", args[0], currentLevel)
		} else {
			fmt.Printf("%s = %s (level %d)\n", args[0], resp.GetValue(), currentLevel)
		}
	}
	cancel()
	time.Sleep(delayOutput)
	fmt.Println("Correctable read finished")
}

func (repl) writeQC(args []string, config pb.Configuration) {
	if len(args) < 2 {
		fmt.Println("Write requires a key and a value to write.")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	cfgCtx := config.Context(ctx)
	// Use the responses iterator to count successful updates
	resp, err := numUpdated(pb.WriteQC(cfgCtx, pb.WriteRequest_builder{
		Key: args[0], Value: args[1], Time: timestamppb.Now(),
	}.Build()))
	cancel()
	if err != nil {
		fmt.Printf("Write RPC finished with error: %v\n", err)
		return
	}
	if !resp.GetNew() {
		fmt.Printf("Failed to update %s: timestamp too old.\n", args[0])
		return
	}
	fmt.Println("Write OK")
}

func (repl) readNestedQC(args []string, config pb.Configuration) {
	if len(args) < 1 {
		fmt.Println("Read requires a key to read.")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	cfgCtx := config.Context(ctx)
	resp, err := newestValue(pb.ReadNestedQC(cfgCtx, pb.ReadRequest_builder{Key: args[0]}.Build()))
	cancel()
	if err != nil {
		fmt.Printf("Nested read quorum call finished with error: %v\n", err)
		return
	}
	if !resp.GetOK() {
		fmt.Printf("%s was not found\n", args[0])
		return
	}
	time.Sleep(delayOutput)
	fmt.Printf("%s = %s\n", args[0], resp.GetValue())
}

func (repl) writeNestedMulticast(args []string, config pb.Configuration) {
	if len(args) < 2 {
		fmt.Println("Write requires a key and a value to write.")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	cfgCtx := config.Context(ctx)
	resp, err := pb.WriteNestedMulticast(cfgCtx, pb.WriteRequest_builder{
		Key: args[0], Value: args[1], Time: timestamppb.Now(),
	}.Build()).Majority()
	cancel()
	if err != nil {
		fmt.Printf("Nested write quorum call finished with error: %v\n", err)
		return
	}
	if !resp.GetNew() {
		fmt.Printf("Failed to update %s in nested multicast path.\n", args[0])
		return
	}
	time.Sleep(delayOutput)
	fmt.Println("Nested write OK")
}

func (r repl) parseConfiguration(cfgStr string) (pb.Configuration, error) {
	indices, err := parseIndices(cfgStr, r.mgr.Size())
	if err != nil {
		return nil, err
	}

	nodes := make([]*pb.Node, 0, len(indices))
	mgrNodes := r.mgr.Nodes()
	for _, i := range indices {
		nodes = append(nodes, mgrNodes[i])
	}
	gorums.OrderedBy(gorums.ID).Sort(nodes)
	return pb.Configuration(nodes), nil
}

func parseIndices(cfgStr string, numNodes int) (indices []int, err error) {
	// configuration using range syntax
	if i := strings.Index(cfgStr, ":"); i > -1 {
		var start, stop int
		if i == 0 {
			start = 0
		} else {
			start, err = strconv.Atoi(cfgStr[:i])
			if err != nil {
				return nil, err
			}
		}
		if i == len(cfgStr)-1 {
			stop = numNodes
		} else {
			stop, err = strconv.Atoi(cfgStr[i+1:])
			if err != nil {
				return nil, err
			}
		}
		if start >= stop || start < 0 || stop > numNodes {
			return nil, fmt.Errorf("invalid configuration")
		}
		indices = make([]int, 0, stop-start)
		for j := start; j < stop; j++ {
			indices = append(indices, j)
		}
		return indices, nil
	}
	// configuration using list of indices
	if indicesStr := strings.Split(cfgStr, ","); len(indicesStr) > 0 {
		indices = make([]int, 0, len(indicesStr))
		for _, indexStr := range indicesStr {
			i, err := strconv.Atoi(indexStr)
			if err != nil {
				return nil, err
			}
			if i < 0 || i >= numNodes {
				return nil, fmt.Errorf("invalid configuration")
			}
			indices = append(indices, i)
		}
		return indices, nil
	}
	return nil, fmt.Errorf("invalid configuration")
}

func splitQuoted(s string) ([]string, error) {
	var args []string
	var currentArg strings.Builder
	var quote rune
	escaped := false
	inArg := false

	flush := func() {
		if inArg {
			args = append(args, currentArg.String())
			currentArg.Reset()
			inArg = false
		}
	}

	for _, r := range s {
		switch {
		case escaped:
			currentArg.WriteRune(r)
			escaped = false
			inArg = true

		case quote != 0:
			inArg = true
			switch r {
			case '\\':
				escaped = true
			case quote:
				quote = 0
			default:
				currentArg.WriteRune(r)
			}

		case r == '\'' || r == '"':
			quote = r
			inArg = true

		case unicode.IsSpace(r):
			flush()

		case r == '\\':
			escaped = true
			inArg = true

		default:
			currentArg.WriteRune(r)
			inArg = true
		}
	}

	if escaped {
		return nil, fmt.Errorf("unfinished escape")
	}
	if quote != 0 {
		return nil, fmt.Errorf("unterminated quoted string")
	}

	flush()
	return args, nil
}
