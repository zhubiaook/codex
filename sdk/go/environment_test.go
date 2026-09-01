package codex

import (
	"testing"
)

func TestSplitEnvironmentEntry(t *testing.T) {
	type result struct {
		key   string
		value string
		ok    bool
	}
	tests := []struct {
		name  string
		entry string
		want  result
	}{
		{name: "ordinary", entry: "NAME=value=with=equals", want: result{key: "NAME", value: "value=with=equals", ok: true}},
		{name: "Windows drive", entry: `=C:=C:\workspace`, want: result{key: "=C:", value: `C:\workspace`, ok: true}},
		{name: "invalid", entry: "NAME", want: result{key: "NAME"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			key, value, ok := splitEnvironmentEntry(test.entry)
			got := result{key: key, value: value, ok: ok}
			if got != test.want {
				t.Errorf("splitEnvironmentEntry(%q) = %#v, want %#v", test.entry, got, test.want)
			}
		})
	}
}
