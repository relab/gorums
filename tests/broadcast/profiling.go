package broadcast

import (
	"os"
	"runtime/trace"
)

func StartTrace(tracePath string) (stop func() error, err error) {
	traceFile, err := os.Create(tracePath)
	if err != nil {
		return nil, err
	}
	if err := trace.Start(traceFile); err != nil {
		return nil, err
	}
	return func() error {
		trace.Stop()
		err = traceFile.Close()
		if err != nil {
			return err
		}
		return nil
	}, nil
}
