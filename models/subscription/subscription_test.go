package subscription

import (
	"reflect"
	"testing"
)

func TestKeywords(t *testing.T) {
	var ss Subscriptions
	ss.AddKeywords("stock", []string{"a", "b"})
	ss.AddKeywords("Stock", []string{"A", "c", ""})
	ss.AddKeywords("movie", []string{"x"})

	s, ok := ss.Find("STOCK")
	if !ok || !reflect.DeepEqual([]string(s.Keywords), []string{"a", "b", "c"}) {
		t.Errorf("stock keywords = %v", s.Keywords)
	}
	if got := ss.StringKeywords(); got != "movie: x\nstock: a, b, c" {
		t.Errorf("StringKeywords() = %q", got)
	}

	ss.RemoveKeywords("stock", []string{"B"})
	if s, _ := ss.Find("stock"); !reflect.DeepEqual([]string(s.Keywords), []string{"a", "c"}) {
		t.Errorf("after remove = %v", s.Keywords)
	}
	ss.RemoveKeywords("stock", []string{"*"})
	if _, ok := ss.Find("stock"); ok {
		t.Error("empty subscription should be removed")
	}
	if got := ss.KeywordBoards(); !reflect.DeepEqual(got, []string{"movie"}) {
		t.Errorf("KeywordBoards() = %v", got)
	}
}

func TestRemoveMissingKeywordKeepsNothingEmpty(t *testing.T) {
	var ss Subscriptions
	ss.RemoveKeywords("stock", []string{"a"})
	if len(ss) != 0 {
		t.Errorf("subscriptions = %v", ss)
	}
}

func TestArticles(t *testing.T) {
	var ss Subscriptions
	ss.AddKeywords("stock", []string{"a"})
	ss.AddArticle("Stock", "M.1.A.1")
	ss.AddArticle("joke", "M.2.A.2")
	ss.AddArticle("joke", "M.2.A.2")
	if ss.ArticleCount() != 2 || !ss.HasArticle("M.1.A.1") {
		t.Errorf("subscriptions = %v", ss)
	}
	want := "joke:\nhttps://www.ptt.cc/bbs/joke/M.2.A.2.html\nstock:\nhttps://www.ptt.cc/bbs/stock/M.1.A.1.html"
	if got := ss.StringArticles(); got != want {
		t.Errorf("StringArticles() = %q", got)
	}
	if !ss.RemoveArticle("M.2.A.2") || ss.RemoveArticle("M.2.A.2") {
		t.Error("RemoveArticle should report tracked only once")
	}
	if _, ok := ss.Find("joke"); ok {
		t.Error("empty subscription should be removed")
	}
	if s, _ := ss.Find("stock"); len(s.Keywords) != 1 || len(s.Articles) != 1 {
		t.Errorf("stock = %v", s)
	}
}
