package main

import (
	"encoding/csv"
	"errors"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"
)

const csvProgressThreshold = 100 << 20

// writeCSV writes a header and one record per row to path.
func writeCSV[T any](path string, header []string, rows []T, fields func(T) []string) (err error) {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() {
		err = errors.Join(err, f.Close())
	}()
	return writeCSVTo(f, header, rows, fields)
}

// writeCSVTo writes a header and one record per row to w.
func writeCSVTo[T any](w io.Writer, header []string, rows []T, fields func(T) []string) error {
	cw := csv.NewWriter(w)
	if err := cw.Write(header); err != nil {
		return err
	}
	for _, row := range rows {
		if err := cw.Write(fields(row)); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

// forEachCSVRow streams the data records in path to visit. The header is
// exposed as a name-to-column map so readers tolerate reordered and additional
// columns. When progress is set, large inputs periodically report bytes read.
func forEachCSVRow(path string, progress bool, visit func([]string, map[string]int) error) (err error) {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() {
		err = errors.Join(err, f.Close())
	}()

	var input io.Reader = f
	var tracker *csvProgressReader
	if progress {
		info, statErr := f.Stat()
		if statErr != nil {
			return statErr
		}
		log.Printf("  report: reading %s (%s)...", path, formatSize(info.Size()))
		if info.Size() >= csvProgressThreshold {
			tracker = newCSVProgressReader(f, filepath.Base(path), info.Size())
			input = tracker
		}
	}

	cr := csv.NewReader(input)
	cr.ReuseRecord = true
	header, err := cr.Read()
	if err == io.EOF {
		return nil
	}
	if err != nil {
		return err
	}
	columns := columnIndex(header)
	rows := 0
	for {
		record, readErr := cr.Read()
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return readErr
		}
		if err := visit(record, columns); err != nil {
			return err
		}
		rows++
	}
	if progress {
		if tracker != nil {
			tracker.finish()
		}
		log.Printf("  report: read %d row(s) from %s", rows, path)
	}
	return nil
}

type csvProgressReader struct {
	reader     io.Reader
	name       string
	total      int64
	read       int64
	step       int64
	next       int64
	lastReport time.Time
	reported   int
}

func newCSVProgressReader(reader io.Reader, name string, total int64) *csvProgressReader {
	step := max(total/10, int64(64<<20))
	return &csvProgressReader{
		reader: reader, name: name, total: total,
		step: step, next: step, lastReport: time.Now(),
	}
}

func (r *csvProgressReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	r.read += int64(n)
	if r.read >= r.next && time.Since(r.lastReport) >= 2*time.Second {
		r.reported = int(100 * r.read / r.total)
		log.Printf("  report: reading %s: %d%%", r.name, r.reported)
		r.next = r.read + r.step
		r.lastReport = time.Now()
	}
	return n, err
}

func (r *csvProgressReader) finish() {
	if r.reported < 100 {
		log.Printf("  report: reading %s: 100%%", r.name)
	}
}
