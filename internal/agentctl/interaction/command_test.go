package interaction

import "testing"

func TestEncodeCommandPromptPreservesDurableMarker(t *testing.T) {
	got := EncodeCommandPrompt("command-42", "ship it")
	want := "[Orkest command \"command-42\"]\n\nship it"
	if got != want {
		t.Fatalf("encoded command marker = %q, want %q", got, want)
	}
}
