package interaction

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestToolDetailsRedactBeforeTruncation(t *testing.T) {
	secret := "credential-with-a-long-value"
	input := `{"padding":"` + strings.Repeat("x", MaxToolFieldBytes-20) + `","password":"hidden-password","path":"` + secret + `"}`
	u := Update{Kind: "tool_call", Title: secret, Input: input, Locations: []ToolLocation{{Path: secret}}, Content: []ToolContent{{Kind: "diff", Path: secret, OldText: secret, NewText: secret}}}
	sanitized := RedactToolUpdate(u, []string{secret})
	b, _ := json.Marshal(sanitized)
	if strings.Contains(string(b), secret) || strings.Contains(string(b), "hidden-password") {
		t.Fatal("credential leaked")
	}
	if u.Locations[0].Path != secret || u.Content[0].Path != secret {
		t.Fatal("redaction mutated its caller's content")
	}
	bounded := BoundToolUpdate(sanitized)
	if !bounded.Truncated || len(bounded.Input) > MaxToolFieldBytes {
		t.Fatal("input was not bounded")
	}
}

func TestToolDetailsBoundMetadataAndUnicode(t *testing.T) {
	u := Update{Kind: "tool_call", Input: strings.Repeat("é", 20000), Output: strings.Repeat("\\\"", 20000)}
	for range 40 {
		u.Locations = append(u.Locations, ToolLocation{Path: strings.Repeat("p", 20000)})
	}
	for range 12 {
		u.Content = append(u.Content, ToolContent{Kind: "diff", Path: "file", OldText: strings.Repeat("é", 20000), NewText: strings.Repeat("n", 20000)})
	}
	u = BoundToolUpdate(u)
	b, err := json.Marshal(u)
	if err != nil || len(b) > MaxToolDetailBytes || !u.Truncated || !utf8.ValidString(u.Input) {
		t.Fatalf("invalid bounded update: size=%d", len(b))
	}
}

func TestToolCollectionsPreserveEmptyVersusOmitted(t *testing.T) {
	for _, tc := range []struct {
		update Update
		want   string
	}{
		{Update{Kind: "tool_call_update"}, `{"kind":"tool_call_update"}`},
		{Update{Kind: "tool_call_update", Locations: []ToolLocation{}, Content: []ToolContent{}}, `{"kind":"tool_call_update","locations":[],"content":[]}`},
	} {
		b, err := json.Marshal(tc.update)
		if err != nil || string(b) != tc.want {
			t.Fatalf("unexpected update: %s", b)
		}
	}
}
