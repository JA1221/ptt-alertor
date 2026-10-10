package board

import (
	"os"
	"reflect"
	"sort"
	"testing"

	"github.com/Ptt-Alertor/ptt-alertor/internal/redistest"
	"github.com/Ptt-Alertor/ptt-alertor/models/article"
)

func TestMain(m *testing.M) {
	os.Exit(redistest.Run(m))
}

func ids(as article.Articles) []int {
	var ids []int
	for _, a := range as {
		ids = append(ids, a.ID)
	}
	return ids
}

func TestNewArticles(t *testing.T) {
	saved := article.Articles{{ID: 0}, {ID: 10}, {ID: 12}}
	online := article.Articles{{ID: 11}, {ID: 0}, {ID: 12}, {ID: 13}, {ID: 14}}
	if got := ids(NewArticles(saved, online)); !reflect.DeepEqual(got, []int{13, 14}) {
		t.Errorf("NewArticles() = %v", got)
	}
	if got := NewArticles(article.Articles{{ID: 0}}, online); len(got) != 0 {
		t.Errorf("saved without ids has nothing to compare, got %v", ids(got))
	}
}

func TestSync(t *testing.T) {
	redistest.Flush()
	for _, name := range []string{"stock", "joke"} {
		if err := Add(name); err != nil {
			t.Fatal(err)
		}
		if err := SaveArticles(name, article.Articles{{ID: 1}}); err != nil {
			t.Fatal(err)
		}
	}
	if err := Sync([]string{"stock", "movie"}); err != nil {
		t.Fatal(err)
	}
	list := List()
	sort.Strings(list)
	if !reflect.DeepEqual(list, []string{"movie", "stock"}) {
		t.Errorf("List() = %v", list)
	}
	if saved, _ := SavedArticles("joke"); saved != nil {
		t.Errorf("joke articles = %v", saved)
	}
	if saved, err := SavedArticles("stock"); err != nil || len(saved) != 1 {
		t.Errorf("stock articles = %v, %v", saved, err)
	}
}

func TestSuggest(t *testing.T) {
	redistest.Flush()
	for _, name := range []string{"gossiping", "stock", "movie"} {
		Add(name)
	}
	tests := map[string]string{"gossipnig": "gossiping", "Stokc": "stock", "abc": ""}
	for input, want := range tests {
		if got := Suggest(input); got != want {
			t.Errorf("Suggest(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestFixLink(t *testing.T) {
	articles := article.Articles{
		{Title: "[問卦] 測試 (Gossiping)", Link: "https://www.ptt.cc/bbs/ALLPOST/M.1.A.1.html"},
		{Title: "no board", Link: "https://www.ptt.cc/bbs/ALLPOST/M.2.A.2.html"},
	}
	fixLink(articles)
	if articles[0].Link != "https://www.ptt.cc/bbs/Gossiping/M.1.A.1.html" || articles[1].Link != "https://www.ptt.cc/bbs/ALLPOST/M.2.A.2.html" {
		t.Errorf("links = %q, %q", articles[0].Link, articles[1].Link)
	}
}
