package ai

import (
	"container/heap"

	"github.com/kasuganosora/rpgmakermvmmo/server/resource"
)

// Point is a 2D grid coordinate.
type Point struct {
	X, Y int
}

// AStar finds the shortest passable path from `from` to `to` on the given passability map.
// Returns the path as a slice of Points (excluding the start, including the end).
// Returns nil if no path exists.
func AStar(pm *resource.PassabilityMap, from, to Point) []Point {
	if pm == nil {
		return nil
	}
	if from == to {
		return []Point{}
	}

	type node struct {
		pt     Point
		g, f   int
		parent *node
		index  int
	}

	heuristic := func(a, b Point) int {
		dx := a.X - b.X
		if dx < 0 {
			dx = -dx
		}
		dy := a.Y - b.Y
		if dy < 0 {
			dy = -dy
		}
		return dx + dy
	}

	// Priority queue.
	type pqItem struct {
		n    *node
		prio int
	}
	var pq []pqItem
	pushPQ := func(n *node) {
		pq = append(pq, pqItem{n, n.f})
		// bubble up
		i := len(pq) - 1
		for i > 0 {
			parent := (i - 1) / 2
			if pq[parent].prio <= pq[i].prio {
				break
			}
			pq[parent], pq[i] = pq[i], pq[parent]
			i = parent
		}
	}
	popPQ := func() *node {
		n := pq[0].n
		last := len(pq) - 1
		pq[0] = pq[last]
		pq = pq[:last]
		// sift down
		i := 0
		for {
			left, right := 2*i+1, 2*i+2
			smallest := i
			if left < len(pq) && pq[left].prio < pq[smallest].prio {
				smallest = left
			}
			if right < len(pq) && pq[right].prio < pq[smallest].prio {
				smallest = right
			}
			if smallest == i {
				break
			}
			pq[i], pq[smallest] = pq[smallest], pq[i]
			i = smallest
		}
		return n
	}
	_ = heap.Interface(nil) // suppress unused import warning

	type key = Point
	closed := make(map[key]bool)
	gScore := make(map[key]int)

	start := &node{pt: from, g: 0, f: heuristic(from, to)}
	gScore[from] = 0
	pushPQ(start)

	dirs := []Point{{0, 1}, {0, -1}, {1, 0}, {-1, 0}}
	dirRMMV := []int{2, 8, 6, 4} // down, up, right, left

	for len(pq) > 0 {
		cur := popPQ()
		if closed[cur.pt] {
			continue
		}
		closed[cur.pt] = true

		if cur.pt == to {
			// Reconstruct path.
			var path []Point
			for n := cur; n.parent != nil; n = n.parent {
				path = append(path, n.pt)
			}
			// Reverse.
			for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
				path[i], path[j] = path[j], path[i]
			}
			return path
		}

		for i, d := range dirs {
			np := Point{cur.pt.X + d.X, cur.pt.Y + d.Y}
			if closed[np] {
				continue
			}
			if !pm.CanPass(cur.pt.X, cur.pt.Y, dirRMMV[i]) {
				continue
			}
			ng := cur.g + 1
			if prev, ok := gScore[np]; !ok || ng < prev {
				gScore[np] = ng
				next := &node{
					pt:     np,
					g:      ng,
					f:      ng + heuristic(np, to),
					parent: cur,
				}
				pushPQ(next)
			}
		}
	}

	return nil // no path found
}
