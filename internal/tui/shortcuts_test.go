package tui

import "testing"

func TestSortShortcutDefsStableAndIndependent(t *testing.T) {
	defs := []shortcutDef{
		{action: "last", order: 2},
		{action: "first", order: 1},
		{action: "second", order: 1},
	}
	sorted := sortShortcutDefs(defs)
	for i, want := range []shortcutAction{"first", "second", "last"} {
		if got := sorted[i].action; got != want {
			t.Errorf("sorted[%d].action = %q, want %q", i, got, want)
		}
	}
	if defs[0].action != "last" {
		t.Fatal("sorting mutated the input slice")
	}
	sorted[0].action = "changed"
	if defs[1].action != "first" {
		t.Fatal("sorted slice shares storage with the input")
	}
	if got := sortShortcutDefs(nil); got != nil {
		t.Errorf("sortShortcutDefs(nil) = %v, want nil", got)
	}
}
