package scanlogic

import "fmt"

// Validate rejects illegal programs before any replay happens.
func (p *Program) Validate() error {
	if len(p.Statements) == 0 {
		return fmt.Errorf("program must contain at least one statement")
	}
	declared := map[string]string{} // name -> kind
	addName := func(name, kind string) error {
		if name == "" {
			return fmt.Errorf("%s: name must not be empty", kind)
		}
		if prev, ok := declared[name]; ok {
			return fmt.Errorf("name %q declared twice (%s and %s)", name, prev, kind)
		}
		declared[name] = kind
		return nil
	}
	for _, n := range p.Inputs {
		if err := addName(n, "input"); err != nil {
			return err
		}
	}
	for _, n := range p.Coils {
		if err := addName(n, "coil"); err != nil {
			return err
		}
	}
	for _, t := range p.Timers {
		if err := addName(t.Name, "timer"); err != nil {
			return err
		}
		if t.Preset <= 0 {
			return fmt.Errorf("timer %q: preset must be a positive integer, got %d", t.Name, t.Preset)
		}
	}
	for _, c := range p.Counters {
		if err := addName(c.Name, "counter"); err != nil {
			return err
		}
		if c.Preset <= 0 {
			return fmt.Errorf("counter %q: preset must be a positive integer, got %d", c.Name, c.Preset)
		}
	}

	// Each state (coil/timer/counter) may be written by at most one statement.
	written := map[string]int{}
	for i, st := range p.Statements {
		target, conds, err := p.statementParts(st)
		if err != nil {
			return fmt.Errorf("statement %d: %w", i, err)
		}
		kind, ok := declared[target]
		if !ok {
			return fmt.Errorf("statement %d: write target %q is not declared", i, target)
		}
		want := map[string]string{"coil": "coil", "ton": "timer", "ctu": "counter"}[stmtKind(st)]
		if kind != want {
			return fmt.Errorf("statement %d: %s statement cannot write %s %q", i, stmtKind(st), kind, target)
		}
		if prev, dup := written[target]; dup {
			return fmt.Errorf("statement %d: %q is already written by statement %d (one writer per scan)", i, target, prev)
		}
		written[target] = i
		for _, c := range conds {
			if err := p.validateCondition(c, declared); err != nil {
				return fmt.Errorf("statement %d: %w", i, err)
			}
		}
	}
	return nil
}

func stmtKind(st Statement) string {
	switch {
	case st.Coil != nil:
		return "coil"
	case st.TON != nil:
		return "ton"
	case st.CTU != nil:
		return "ctu"
	}
	return ""
}

// statementParts returns the write target and all conditions of a statement.
func (p *Program) statementParts(st Statement) (string, []Condition, error) {
	n := 0
	for _, set := range []bool{st.Coil != nil, st.TON != nil, st.CTU != nil} {
		if set {
			n++
		}
	}
	if n != 1 {
		return "", nil, fmt.Errorf("exactly one of coil, ton, ctu must be set")
	}
	switch {
	case st.Coil != nil:
		return st.Coil.Target, []Condition{st.Coil.Cond}, nil
	case st.TON != nil:
		return st.TON.Timer, []Condition{st.TON.Cond}, nil
	default:
		return st.CTU.Counter, []Condition{st.CTU.Cond, st.CTU.Reset}, nil
	}
}

func (p *Program) validateCondition(c Condition, declared map[string]string) error {
	n := 0
	for _, set := range []bool{c.Contact != "", c.Not != nil, len(c.And) > 0, len(c.Or) > 0} {
		if set {
			n++
		}
	}
	if n != 1 {
		return fmt.Errorf("condition must set exactly one of contact, not, and, or")
	}
	if c.Contact != "" {
		if _, ok := declared[c.Contact]; !ok {
			return fmt.Errorf("condition references undeclared name %q", c.Contact)
		}
		return nil
	}
	if c.Not != nil {
		return p.validateCondition(*c.Not, declared)
	}
	for _, sub := range append(append([]Condition{}, c.And...), c.Or...) {
		if err := p.validateCondition(sub, declared); err != nil {
			return err
		}
	}
	return nil
}
