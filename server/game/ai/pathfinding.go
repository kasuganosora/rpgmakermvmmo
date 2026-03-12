package ai

import (
	"sync"

	"github.com/kasuganosora/rpgmakermvmmo/server/resource"
)

// Point is a 2D grid coordinate.
type Point struct {
	X, Y int
}

// maxAStarCost limits the search radius to prevent runaway pathfinding.
const maxAStarCost = 200

// astarState is a reusable scratch buffer for A* searches.
// Pooled to avoid per-call allocations of large flat arrays.
type astarState struct {
	closed []bool // flat w*h array: closed[y*w+x]
	gScore []int  // flat w*h array: gScore[y*w+x] (-1 = unvisited)
	pq     []pqItem
	nodes  []astarNode
	w, h   int
}

type pqItem struct {
	nodeIdx int
	prio    int
}

type astarNode struct {
	pt        Point
	g, f      int
	parentIdx int // -1 = no parent
}

var astarPool = sync.Pool{
	New: func() interface{} {
		return &astarState{}
	},
}

func (s *astarState) reset(w, h int) {
	size := w * h
	s.w = w
	s.h = h

	if cap(s.closed) < size {
		s.closed = make([]bool, size)
		s.gScore = make([]int, size)
	} else {
		s.closed = s.closed[:size]
		s.gScore = s.gScore[:size]
	}
	for i := range s.closed {
		s.closed[i] = false
		s.gScore[i] = -1
	}
	s.pq = s.pq[:0]
	s.nodes = s.nodes[:0]
}

func (s *astarState) idx(x, y int) int { return y*s.w + x }

func (s *astarState) pushPQ(nodeIdx int) {
	s.pq = append(s.pq, pqItem{nodeIdx, s.nodes[nodeIdx].f})
	i := len(s.pq) - 1
	for i > 0 {
		parent := (i - 1) / 2
		if s.pq[parent].prio <= s.pq[i].prio {
			break
		}
		s.pq[parent], s.pq[i] = s.pq[i], s.pq[parent]
		i = parent
	}
}

func (s *astarState) popPQ() int {
	top := s.pq[0].nodeIdx
	last := len(s.pq) - 1
	s.pq[0] = s.pq[last]
	s.pq = s.pq[:last]
	i := 0
	for {
		left, right := 2*i+1, 2*i+2
		smallest := i
		if left < len(s.pq) && s.pq[left].prio < s.pq[smallest].prio {
			smallest = left
		}
		if right < len(s.pq) && s.pq[right].prio < s.pq[smallest].prio {
			smallest = right
		}
		if smallest == i {
			break
		}
		s.pq[i], s.pq[smallest] = s.pq[smallest], s.pq[i]
		i = smallest
	}
	return top
}

func (s *astarState) addNode(pt Point, g, f, parentIdx int) int {
	idx := len(s.nodes)
	s.nodes = append(s.nodes, astarNode{pt: pt, g: g, f: f, parentIdx: parentIdx})
	return idx
}

// heuristicWrap computes Manhattan distance accounting for map wrapping.
func heuristicWrap(a, b Point, w, h int, loopH, loopV bool) int {
	dx := a.X - b.X
	if dx < 0 {
		dx = -dx
	}
	if loopH && dx > w/2 {
		dx = w - dx
	}
	dy := a.Y - b.Y
	if dy < 0 {
		dy = -dy
	}
	if loopV && dy > h/2 {
		dy = h - dy
	}
	return dx + dy
}

// AStar finds the shortest passable path from `from` to `to` on the given passability map.
// Returns the path as a slice of Points (excluding the start, including the end).
// Returns nil if no path exists. Supports map wrapping (looping maps).
func AStar(pm *resource.PassabilityMap, from, to Point) []Point {
	if pm == nil {
		return nil
	}
	if from == to {
		return []Point{}
	}

	loopH, loopV := pm.IsLoopH(), pm.IsLoopV()

	st := astarPool.Get().(*astarState)
	defer astarPool.Put(st)
	st.reset(pm.Width, pm.Height)

	startIdx := st.addNode(from, 0, heuristicWrap(from, to, pm.Width, pm.Height, loopH, loopV), -1)
	st.gScore[st.idx(from.X, from.Y)] = 0
	st.pushPQ(startIdx)

	type dir struct {
		dx, dy, rmmv int
	}
	dirs := [4]dir{{0, 1, 2}, {0, -1, 8}, {1, 0, 6}, {-1, 0, 4}}

	for len(st.pq) > 0 {
		curIdx := st.popPQ()
		cur := &st.nodes[curIdx]
		flatIdx := st.idx(cur.pt.X, cur.pt.Y)
		if st.closed[flatIdx] {
			continue
		}
		st.closed[flatIdx] = true

		if cur.pt == to {
			// Reconstruct path.
			var path []Point
			for ni := curIdx; st.nodes[ni].parentIdx != -1; ni = st.nodes[ni].parentIdx {
				path = append(path, st.nodes[ni].pt)
			}
			for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
				path[i], path[j] = path[j], path[i]
			}
			return path
		}

		if cur.g >= maxAStarCost {
			continue // don't explore beyond max range
		}

		for _, d := range dirs {
			nx, ny := cur.pt.X+d.dx, cur.pt.Y+d.dy
			// Apply coordinate wrapping for looping maps.
			nx = pm.RoundX(nx)
			ny = pm.RoundY(ny)
			if !pm.IsValid(nx, ny) {
				continue
			}
			nFlat := st.idx(nx, ny)
			if st.closed[nFlat] {
				continue
			}
			// CanPass already handles coordinate wrapping internally.
			if !pm.CanPass(cur.pt.X, cur.pt.Y, d.rmmv) {
				continue
			}
			ng := cur.g + 1
			if prev := st.gScore[nFlat]; prev == -1 || ng < prev {
				st.gScore[nFlat] = ng
				np := Point{nx, ny}
				ni := st.addNode(np, ng, ng+heuristicWrap(np, to, pm.Width, pm.Height, loopH, loopV), curIdx)
				st.pushPQ(ni)
			}
		}
	}

	return nil // no path found
}
