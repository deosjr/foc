package mock

import "testing"

func TestSplitCondition(t *testing.T) {
	cases := []struct{ letter, rest, cond, then string }{
		{"watch the ford, and strike if they try to cross.", "watch the ford,.", "if they try to cross", "and strike"},
		{"hold hollow wood if you can, but fall back if they come in force.", "hold hollow wood if you can,.", "if they come in force", "but fall back"},
		{"if duna hills falls, march to karsa.", ".", "if duna hills falls", ", march to karsa"},
		{"march on marren.", "march on marren.", "", ""},
	}
	for _, c := range cases {
		rest, cond, then := splitCondition(c.letter)
		if rest != c.rest || cond != c.cond || then != c.then {
			t.Errorf("%q: got rest=%q cond=%q then=%q", c.letter, rest, cond, then)
		}
	}
}
