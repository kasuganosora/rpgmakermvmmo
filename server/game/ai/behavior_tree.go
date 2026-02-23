package ai

// Status is the result of a behavior tree node tick.
type Status int

const (
	StatusSuccess Status = iota
	StatusFailure
	StatusRunning
)

// Node is a single node in a behavior tree.
type Node interface {
	Tick(ctx *AIContext) Status
}

// ---- Composite nodes ----

// Selector succeeds as soon as one child succeeds (logical OR).
type Selector struct {
	Children []Node
}

func (s *Selector) Tick(ctx *AIContext) Status {
	for _, c := range s.Children {
		switch c.Tick(ctx) {
		case StatusSuccess:
			return StatusSuccess
		case StatusRunning:
			return StatusRunning
		}
	}
	return StatusFailure
}

// Sequence succeeds only when all children succeed (logical AND).
type Sequence struct {
	Children []Node
}

func (s *Sequence) Tick(ctx *AIContext) Status {
	for _, c := range s.Children {
		switch c.Tick(ctx) {
		case StatusFailure:
			return StatusFailure
		case StatusRunning:
			return StatusRunning
		}
	}
	return StatusSuccess
}

// ---- Leaf nodes ----

// ConditionNode evaluates a boolean predicate.
type ConditionNode struct {
	Fn func(*AIContext) bool
}

func (cn *ConditionNode) Tick(ctx *AIContext) Status {
	if cn.Fn(ctx) {
		return StatusSuccess
	}
	return StatusFailure
}

// ActionNode executes an action and returns its status.
type ActionNode struct {
	Fn func(*AIContext) Status
}

func (an *ActionNode) Tick(ctx *AIContext) Status {
	return an.Fn(ctx)
}

// ---- Decorator nodes ----

// Inverter negates the result of its child.
type Inverter struct {
	Child Node
}

func (i *Inverter) Tick(ctx *AIContext) Status {
	switch i.Child.Tick(ctx) {
	case StatusSuccess:
		return StatusFailure
	case StatusFailure:
		return StatusSuccess
	default:
		return StatusRunning
	}
}

// ---- BehaviorTree root ----

// BehaviorTree wraps the root node.
type BehaviorTree struct {
	Root Node
}

// Tick runs one frame of the behavior tree.
func (bt *BehaviorTree) Tick(ctx *AIContext) Status {
	if bt.Root == nil {
		return StatusFailure
	}
	return bt.Root.Tick(ctx)
}
