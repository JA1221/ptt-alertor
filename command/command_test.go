package command

import (
	"strings"
	"testing"
)

func TestHandleCommandBlankText(t *testing.T) {
	for _, text := range []string{"", " ", "\t\n"} {
		if got := HandleCommand(text, "tester", false); !strings.Contains(got, "可使用的指令清單") {
			t.Errorf("HandleCommand(%q) = %q, want commands list", text, got)
		}
	}
}
