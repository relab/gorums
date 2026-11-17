package impl

import (
	"testing"
	"testing/synctest"

	pb "google.golang.org/protobuf/types/known/wrapperspb"
	"pgregory.net/rapid"
)

type propertyStarter struct{ dispatched bool }

func (s *propertyStarter) markDispatched() { s.dispatched = true }
func (*propertyStarter) sendNow()          {}

func TestCorrectableProperties(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		rows := responseList.Draw(t, "responses")
		threshold := rapid.IntRange(1, 10).Draw(t, "threshold")
		rapid.SyncTest(t, func(t *rapid.T) {
			input := make(chan NodeResponse[*pb.Int32Value])
			stop := make(chan struct{})
			defer close(stop)
			starter := new(propertyStarter)
			responses := &Responses[*pb.Int32Value]{size: 8, start: starter, seq: func(yield func(NodeResponse[*pb.Int32Value]) bool) {
				for {
					select {
					case r, ok := <-input:
						if !ok || !yield(r) {
							return
						}
					case <-stop:
						return
					}
				}
			}}
			corr := responses.Correctable(threshold)
			if !starter.dispatched {
				t.Fatal("Correctable did not mark dispatch")
			}
			type watch struct {
				level int
				ch    <-chan struct{}
			}
			var watches []watch
			level := LevelNotSet
			var latest *pb.Int32Value
			complete := false
			check := func() {
				t.Helper()
				value, gotLevel, _ := corr.Get()
				if value != latest || gotLevel != level {
					t.Fatalf("Get = (%v, %d), want (%v, %d)", value, gotLevel, latest, level)
				}
				for _, w := range append(watches, watch{threshold, corr.Done()}) {
					closed := false
					select {
					case <-w.ch:
						closed = true
					default:
					}
					if want := complete || level >= w.level; closed != want {
						t.Fatalf("watch(%d) closed=%v at level %d, complete=%v", w.level, closed, level, complete)
					}
				}
			}
			addWatch := func() {
				w := rapid.IntRange(-1, threshold+2).Draw(t, "watch")
				watches = append(watches, watch{w, corr.Watch(w)})
			}
			addWatch()
			check()
			consumed := 0
			for _, row := range rows {
				addWatch()
				input <- row
				consumed++
				if row.Err == nil {
					level = max(level, 0) + 1
					latest = row.Value
					complete = level >= threshold
				}
				synctest.Wait()
				check()
				if complete {
					break
				}
			}
			close(input)
			synctest.Wait()
			reached := complete
			complete = true
			level = max(level, 0)
			addWatch() // Watches created after completion must close immediately too.
			check()
			_, _, err := corr.Get()
			checkTerminalError(t, err, reached, rows[:consumed])
		})
	})
}
