package notify

import "testing"

func TestPlain(t *testing.T) {
	got := Plain("✅ 開始追蹤\n" + Link("a<b>&c", "https://x?a=1&b=2"))
	if got != "✅ 開始追蹤\na<b>&c" {
		t.Errorf("Plain() = %q", got)
	}
}

func TestRelay(t *testing.T) {
	var r Relay
	if err := r.Send(1, Text("x")); err == nil {
		t.Error("send before set should fail")
	}
}
