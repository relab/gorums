package conn

import (
	"cmp"
	"context"
	"slices"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"pgregory.net/rapid"
)

func TestConfigProperties(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		ids := rapid.SliceOfNDistinct(rapid.Uint32Range(1, 32), 0, 16, rapid.ID).Draw(t, "pool")
		// These nodes exercise list operations only: the pool has no transports,
		// listeners, or background goroutines.
		mgr := &outboundManager{lookup: map[ID]*Node{}}
		var a, b Config
		for _, id := range ids {
			node := newNode(id, "", mgr, nil)
			mgr.lookup[id] = node
			if rapid.Bool().Draw(t, "inA") {
				a = append(a, node)
			}
			if rapid.Bool().Draw(t, "inB") {
				b = append(b, node)
			}
		}
		beforeA, beforeB := slices.Clone(a), slices.Clone(b)
		contains := func(c Config, id ID) bool {
			for _, node := range c {
				if node.id == id {
					return true
				}
			}
			return false
		}
		checkMembers := func(got Config, want func(ID) bool) {
			t.Helper()
			seen := map[ID]bool{}
			for _, node := range got {
				if seen[node.id] || mgr.lookup[node.id] != node || !want(node.id) {
					t.Fatalf("unexpected result member %d", node.id)
				}
				seen[node.id] = true
			}
			for _, id := range ids {
				if seen[id] != want(id) {
					t.Fatalf("membership of %d = %v", id, seen[id])
				}
			}
		}
		union := a.Union(b)
		checkMembers(union, func(id ID) bool { return contains(a, id) || contains(b, id) })
		difference := a.Difference(b)
		checkMembers(difference, func(id ID) bool { return contains(a, id) && !contains(b, id) })
		var expected Config
		for _, node := range a {
			if !contains(b, node.id) {
				expected = append(expected, node)
			}
		}
		if !slices.Equal(difference, expected) {
			t.Fatal("difference changed retained order")
		}
		selected := rapid.SliceOfN(rapid.Uint32Range(0, 35), 0, 20).Draw(t, "selectedIDs")
		added := a.Add(selected...)
		checkMembers(added, func(id ID) bool { return len(a) > 0 && (contains(a, id) || slices.Contains(selected, id)) })
		if !slices.IsSorted(added.NodeIDs()) {
			t.Fatal("Add did not sort by ID")
		}
		removed := a.Remove(selected...)
		expected = nil
		for _, node := range a {
			if !slices.Contains(selected, node.id) {
				expected = append(expected, node)
			}
		}
		if !slices.Equal(removed, expected) {
			t.Fatal("Remove changed membership or retained order")
		}
		sorted := a.Sort(func(x, y *Node) int { return cmp.Compare(x.id%3, y.id%3) })
		expected = nil
		for group := range ID(3) {
			for _, node := range a {
				if node.id%3 == group {
					expected = append(expected, node)
				}
			}
		}
		if !slices.Equal(sorted, expected) {
			t.Fatal("Sort violated stable grouping")
		}
		if !a.Equal(slices.Clone(a)) {
			t.Fatal("Equal rejected a copy")
		}
		if len(a) > 1 {
			swapped := slices.Clone(a)
			swapped[0], swapped[1] = swapped[1], swapped[0]
			if a.Equal(swapped) {
				t.Fatal("Equal ignored order")
			}
		}
		for id := range ID(36) {
			if a.Contains(id) != contains(a, id) {
				t.Fatalf("Contains(%d) disagrees with list", id)
			}
			want := mgr.lookup[id]
			if !contains(a, id) {
				want = nil
			}
			if a.Node(id) != want {
				t.Fatalf("Node(%d) disagrees with list", id)
			}
		}
		// Results must have independent slice storage, including identity operations.
		for _, result := range []Config{union, difference, added, removed, sorted, a.Union(nil), a.Difference(nil), a.Remove()} {
			if len(result) > 0 {
				result[0] = nil
			}
			if !slices.Equal(a, beforeA) || !slices.Equal(b, beforeB) {
				t.Fatal("result aliases an input slice")
			}
		}
	})
}

func TestConfigWatchProperties(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		choices := rapid.SliceOfN(rapid.IntRange(0, 3), 1, 20).Draw(t, "views")
		rapid.SyncTest(t, func(t *rapid.T) {
			nodes := Config{NewNodeForTest(1, nil), NewNodeForTest(2, nil)}
			views := []Config{nil, nodes[:1], nodes, {nodes[1], nodes[0]}}
			current := views[0]
			var selected atomic.Int32
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			updates := nodes.Watch(ctx, time.Second, func(Config) Config { return views[selected.Load()] })
			synctest.Wait()
			if initial := <-updates; !initial.Equal(current) {
				t.Fatal("incorrect initial view")
			}
			for _, choice := range choices {
				previous := current
				current = views[choice]
				selected.Store(int32(choice))
				time.Sleep(time.Second)
				synctest.Wait()
				select {
				case got := <-updates:
					if previous.Equal(current) || !got.Equal(current) {
						t.Fatalf("update = %v for %v -> %v", got.NodeIDs(), previous.NodeIDs(), current.NodeIDs())
					}
				default:
					if !previous.Equal(current) {
						t.Fatal("missing changed view")
					}
				}
			}
			cancel()
			synctest.Wait()
			select {
			case _, ok := <-updates:
				if ok {
					t.Fatal("update channel remained open")
				}
			default:
				t.Fatal("Watch did not stop")
			}
		})
	})
}
