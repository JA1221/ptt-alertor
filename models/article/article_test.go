package article

import "testing"

func TestMatchKeyword(t *testing.T) {
	a := Article{Title: "[新聞] 台積電法說會 ABC"}
	tests := map[string]bool{
		"台積電":              true,
		"abc":              true,
		"聯發科":              false,
		"台積電&法說":           true,
		"台積電&聯發科":          false,
		"台積電&!公告":          true,
		"!新聞":              false,
		"regexp:^\\[新聞\\]": true,
		"regexp:^\\[公告\\]": false,
		"regexp:[":         false,
	}
	for kw, want := range tests {
		if got := a.MatchKeyword(kw); got != want {
			t.Errorf("MatchKeyword(%q) = %v, want %v", kw, got, want)
		}
	}
}

func TestParseID(t *testing.T) {
	if id := (Article{}).ParseID("https://www.ptt.cc/bbs/Stock/M.1497363598.A.74E.html"); id != 1497363598 {
		t.Errorf("ParseID() = %d", id)
	}
	if id := (Article{}).ParseID(""); id != 0 {
		t.Errorf("ParseID() = %d", id)
	}
}

func TestNewComments(t *testing.T) {
	online := Article{Comments: Comments{{Content: "a"}, {Content: "b"}, {Content: "c"}}}
	if got := NewComments(Article{CommentCount: 1}, online); len(got) != 2 || got[0].Content != "b" {
		t.Errorf("NewComments() = %v", got)
	}
	if got := NewComments(Article{CommentCount: 3}, online); len(got) != 0 {
		t.Errorf("NewComments() = %v", got)
	}
	if got := NewComments(Article{CommentCount: 5}, online); len(got) != 0 {
		t.Errorf("NewComments() = %v", got)
	}
}
