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

	"github.com/golang/protobuf/ptypes"
	"github.com/google/shlex"
	"github.com/relab/gorums/examples/storage/proto"
	"golang.org/x/crypto/ssh/terminal"
)

var help = `
This interface allows you to run RPCs and quorum calls against the Storage
Servers interactively. Take a look at the files 'client.go' and 'server.go'
for the source code of the RPC handlers and quorum functions.
The following commands can be used:

help 	                        	Show this text
exit 	                        	Exit the program
nodes	                        	Print a list of the available nodes
rpc  	[node index] [operation]	Executes an RPC on the given node.
qc   	[operation]             	Executes a quorum call on all nodes.
cfg  	[config] [opertation]   	Executes a quorum call on a configuration.

The following operations are supported:

read 	[key]        	Read a value
write	[key] [value]	Write a value

Examples:

> rpc 0 write foo bar
The command performs the 'write' RPC on node 0, and sets 'foo' = 'bar'

> qc read foo
The command performs the 'read' quorum call, and returns the value of 'foo'

> cfg 1:3 write foo bar
The command performs the write qourum call on node 1 and 2

> cfg 0,2 write foo 'bar baz'
The command performs the write quorum call on node 0 and 2
`

type repl struct {
	mgr  *proto.Manager
	cfg  *proto.Configuration
	term *terminal.Terminal
}

func newRepl(mgr *proto.Manager, cfg *proto.Configuration) *repl {
	return &repl{
		mgr: mgr,
		cfg: cfg,
		term: terminal.NewTerminal(struct {
			io.Reader
			io.Writer
		}{os.Stdin, os.Stderr}, "> "),
	}
}

// NOTE: x/crypto/ssh/terminal is moving to x/terminal at some point.
// See: https://github.com/golang/go/issues/31044 and https://github.com/golang/term
//
// FIXME: ReadLine currently does not work with arrow keys on windows for some reason
// See: https://stackoverflow.com/questions/58237670/terminal-raw-mode-does-not-support-arrows-on-windows
//
// ReadLine reads a line from the terminal in raw mode
func (r repl) ReadLine() (string, error) {
	fd := int(os.Stdin.Fd())
	oldState, err := terminal.MakeRaw(fd)
	if err != nil {
		panic(err)
	}
	defer func() {
		err := terminal.Restore(fd, oldState)
		if err != nil {
			panic(err)
		}
	}()

	return r.term.ReadLine()
}

func Repl(mgr *proto.Manager, defaultCfg *proto.Configuration) {
	r := newRepl(mgr, defaultCfg)

	fmt.Println(help)
	for {
		l, err := r.ReadLine()
		if errors.Is(err, io.EOF) {
			return
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to read line: %v\n", err)
			os.Exit(1)
		}
		args, err := shlex.Split(l)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to split command: %v\n", err)
			os.Exit(1)
		}
		if len(args) < 1 {
			continue
		}

		switch args[0] {
		case "exit":
			fallthrough
		case "quit":
			return
		case "help":
			fmt.Println(help)
		case "rpc":
			r.rpc(args[1:])
		case "qc":
			r.qc(args[1:])
		case "cfg":
			r.qcCfg(args[1:])
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
		fmt.Printf("Invalid index. Must be between 0 and %d.\n", r.cfg.Size())
		return
	}

	node := r.cfg.Nodes()[index]

	switch args[1] {
	case "read":
		r.readRPC(args[2:], node)
	case "write":
		r.writeRPC(args[2:], node)
	}
}

func (r repl) qc(args []string) {
	if len(args) < 1 {
		fmt.Println("'qc' requires an operation.")
		return
	}

	switch args[0] {
	case "read":
		r.readQC(args[1:], r.cfg)
	case "write":
		r.writeQC(args[1:], r.cfg)
	}
}

func (r repl) qcCfg(args []string) {
	if len(args) < 2 {
		fmt.Println("'cfg' requires a configuration and an operation.")
		return
	}
	cfg := r.parseConfiguration(args[0])
	if cfg == nil {
		return
	}
	switch args[1] {
	case "read":
		r.readQC(args[2:], cfg)
	case "write":
		r.writeQC(args[2:], cfg)
	}
}

func (_ repl) readRPC(args []string, node *proto.Node) {
	if len(args) < 1 {
		fmt.Println("Read requires a key to read.")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	resp, err := node.ReadRPC(ctx, &proto.ReadRequest{Key: args[0]})
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

func (_ repl) writeRPC(args []string, node *proto.Node) {
	if len(args) < 2 {
		fmt.Println("Write requires a key and a value to write.")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	resp, err := node.WriteRPC(ctx, &proto.WriteRequest{Key: args[0], Value: args[1], Time: ptypes.TimestampNow()})
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

func (_ repl) readQC(args []string, cfg *proto.Configuration) {
	if len(args) < 1 {
		fmt.Println("Read requires a key to read.")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	resp, err := cfg.ReadQC(ctx, &proto.ReadRequest{Key: args[0]})
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

func (_ repl) writeQC(args []string, cfg *proto.Configuration) {
	if len(args) < 2 {
		fmt.Println("Write requires a key and a value to write.")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	resp, err := cfg.WriteQC(ctx, &proto.WriteRequest{Key: args[0], Value: args[1], Time: ptypes.TimestampNow()})
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

func (r repl) parseConfiguration(cfgStr string) (cfg *proto.Configuration) {
	// configuration using range syntax
	if i := strings.Index(cfgStr, ":"); i > -1 {
		var start, stop int
		var err error
		numNodes, _ := r.mgr.Size()
		if i == 0 {
			start = 0
		} else {
			start, err = strconv.Atoi(cfgStr[:i])
			if err != nil {
				fmt.Printf("Failed to parse configuration: %v\n", err)
				return nil
			}
		}
		if i == len(cfgStr)-1 {
			stop = numNodes
		} else {
			stop, err = strconv.Atoi(cfgStr[i+1:])
			if err != nil {
				fmt.Printf("Failed to parse configuration: %v\n", err)
				return nil
			}
		}
		if start >= stop || start < 0 || stop >= numNodes {
			fmt.Println("Invalid configuration.")
			return nil
		}
		cfg, err = r.mgr.NewConfiguration(r.mgr.NodeIDs()[start:stop], &qspec{cfgSize: stop - start})
		if err != nil {
			fmt.Printf("Failed to create configuration: %v\n", err)
			return nil
		}
		return cfg
	}
	// configuration using list of indices
	if indices := strings.Split(cfgStr, ","); len(indices) > 0 {
		selectedNodes := make([]uint32, 0, len(indices))
		nodeIDs := r.mgr.NodeIDs()
		for _, index := range indices {
			i, err := strconv.Atoi(index)
			if err != nil {
				fmt.Printf("Failed to parse configuration: %v\n", err)
				return nil
			}
			if i < 0 || i >= len(nodeIDs) {
				fmt.Println("Invalid configuration.")
				return nil
			}
			selectedNodes = append(selectedNodes, nodeIDs[i])
		}
		cfg, err := r.mgr.NewConfiguration(selectedNodes, &qspec{cfgSize: len(selectedNodes)})
		if err != nil {
			fmt.Printf("Failed to create configuration: %v\n", err)
			return nil
		}
		return cfg
	}
	fmt.Println("Invalid configuration.")
	return nil
}
