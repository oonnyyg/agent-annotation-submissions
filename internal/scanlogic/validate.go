package scanlogic

import "fmt"

// Validate rejects illegal programs before any replay happens.
// Rules: presets are positive; every referenced name is declared;
// every writable state (coil, timer, counter) is written by exactly
// one statement; contacts reference inputs or coils, done flags
// reference timers or counters.
func (p *Program) Validate() error {
	inputs := nameSet("input", p.Inputs)
	coils := nameSet("coil", p.Coils)
	timers := map[string]bool{}
	counters := map[string]bool{}
	for _, t := range p.Timers {
		if t.PresetMS <= 0 {
			return fmt.Errorf("timer %q: preset_ms must be a positive integer", t.Name)
		}
		if err := declare(timers, "timer", t.Name); err != nil {
			return err
		}
	}
	for _, c := range p.Counters {
		if c.Preset <= 0 {
			return fmt.Errorf("counter %q: preset must be a positive integer", c.Name)
		}
		if err := declare(counters, "counter", c.Name); err != nil {
			return err
		}
	}
	for kind, set := range map[string]map[string]bool{"input": inputs, "coil": coils, "timer": timers, "counter": counters} {
		for name := range set {
			if err := checkName(kind, name); err != nil {
				return err
			}
		}
	}

	written := map[string]int{}
	for i, st := range p.Statements {
		target, err := st.writesTo(p, coils, timers, counters)
		if err != nil {
			return fmt.Errorf("statement %d: %w", i, err)
		}
		if prev, dup := written[target]; dup {
			return fmt.Errorf("statement %d: %q is already written by statement %d", i, target, prev)
		}
		written[target] = i
		if st.Cond == nil {
			return fmt.Errorf("statement %d: missing condition", i)
		}
		if err := validateCond(st.Cond, inputs, coils, timers, counters); err != nil {
			return fmt.Errorf("statement %d condition: %w", i, err)
		}
		if st.Type == "ctu" {
			if st.Reset == nil {
				return fmt.Errorf("statement %d: ctu requires a reset condition", i)
			}
			if err := validateCond(st.Reset, inputs, coils, timers, counters); err != nil {
				return fmt.Errorf("statement %d reset: %w", i, err)
			}
		} else if st.Reset != nil {
			return fmt.Errorf("statement %d: reset is only valid for ctu", i)
		}
	}
	return nil
}

func nameSet(kind string, names []string) map[string]bool {
	set := map[string]bool{}
	for _, n := range names {
		set[n] = true
		_ = kind
	}
	return set
}

func declare(set map[string]bool, kind, name string) error {
	if set[name] {
		return fmt.Errorf("duplicate %s declaration %q", kind, name)
	}
	set[name] = true
	return nil
}

func checkName(kind, name string) error {
	if name == "" {
		return fmt.Errorf("%s name must not be empty", kind)
	}
	return nil
}

// writesTo returns the state name this statement writes, checking the
// statement shape and that the target is declared.
func (st *Statement) writesTo(p *Program, coils, timers, counters map[string]bool) (string, error) {
	switch st.Type {
	case "coil":
		if st.Target == "" || !coils[st.Target] {
			return "", fmt.Errorf("coil statement targets undeclared coil %q", st.Target)
		}
		return "coil:" + st.Target, nil
	case "ton":
		if st.Timer == "" || !timers[st.Timer] {
			return "", fmt.Errorf("ton statement uses undeclared timer %q", st.Timer)
		}
		return "timer:" + st.Timer, nil
	case "ctu":
		if st.Counter == "" || !counters[st.Counter] {
			return "", fmt.Errorf("ctu statement uses undeclared counter %q", st.Counter)
		}
		return "counter:" + st.Counter, nil
	default:
		return "", fmt.Errorf("unknown statement type %q", st.Type)
	}
}

func validateCond(c *Condition, inputs, coils, timers, counters map[string]bool) error {
	switch c.Op {
	case "contact":
		if !inputs[c.Name] && !coils[c.Name] {
			return fmt.Errorf("contact references undeclared input or coil %q", c.Name)
		}
	case "done":
		if !timers[c.Name] && !counters[c.Name] {
			return fmt.Errorf("done references undeclared timer or counter %q", c.Name)
		}
	case "and", "or":
		if len(c.Args) == 0 {
			return fmt.Errorf("%s requires at least one argument", c.Op)
		}
		for _, a := range c.Args {
			if a == nil {
				return fmt.Errorf("%s has a null argument", c.Op)
			}
			if err := validateCond(a, inputs, coils, timers, counters); err != nil {
				return err
			}
		}
	case "not":
		if len(c.Args) != 1 || c.Args[0] == nil {
			return fmt.Errorf("not requires exactly one argument")
		}
		return validateCond(c.Args[0], inputs, coils, timers, counters)
	default:
		return fmt.Errorf("unknown condition op %q", c.Op)
	}
	return nil
}
