package keyword

import (
	"os"
	"reflect"
	"sort"
	"testing"

	"github.com/Ptt-Alertor/ptt-alertor/internal/redistest"
	"github.com/Ptt-Alertor/ptt-alertor/models/article"
	"github.com/Ptt-Alertor/ptt-alertor/models/board"
	"github.com/Ptt-Alertor/ptt-alertor/models/user"
)

func TestMain(m *testing.M) {
	os.Exit(redistest.Run(m))
}

func edit(t *testing.T, account string, fn func(u *user.User)) {
	t.Helper()
	if _, err := user.Register(account, 0, ""); err != nil {
		t.Fatal(err)
	}
	err := user.Update(account, func(u *user.User) error {
		fn(u)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func subscribe(t *testing.T, account, boardName string, keywords ...string) {
	t.Helper()
	edit(t, account, func(u *user.User) { u.Subscribes.AddKeywords(boardName, keywords) })
}

func subscribers(boardName string) []string {
	subs, _ := Subscribers(boardName)
	sort.Strings(subs)
	return subs
}

func boards() []string {
	list := board.List()
	sort.Strings(list)
	return list
}

func TestRebuild(t *testing.T) {
	redistest.Flush()
	subscribe(t, "1", "stock", "a")
	subscribe(t, "2", "Stock", "b")
	subscribe(t, "2", "joke", "c")
	subscribe(t, "3", "movie", "d")
	if err := board.Add("old"); err != nil {
		t.Fatal(err)
	}
	if err := board.SaveArticles("old", article.Articles{{ID: 1}}); err != nil {
		t.Fatal(err)
	}

	// 3 is not a member
	if err := Rebuild([]string{"1", "2", "404"}); err != nil {
		t.Fatal(err)
	}
	if got := subscribers("STOCK"); !reflect.DeepEqual(got, []string{"1", "2"}) {
		t.Errorf("stock subscribers = %v", got)
	}
	if got := boards(); !reflect.DeepEqual(got, []string{"joke", "stock"}) {
		t.Errorf("boards = %v", got)
	}
	if saved, _ := board.SavedArticles("old"); saved != nil {
		t.Error("saved articles of unsubscribed board should be removed")
	}
}

func TestUpdate(t *testing.T) {
	redistest.Flush()
	subscribe(t, "1", "stock", "a")
	subscribe(t, "2", "stock", "b")
	subscribe(t, "2", "joke", "c")
	if err := Rebuild([]string{"1", "2"}); err != nil {
		t.Fatal(err)
	}

	// last keyword of joke removed
	edit(t, "2", func(u *user.User) { u.Subscribes.RemoveKeywords("joke", []string{"*"}) })
	subscribe(t, "2", "movie", "d")
	if err := Update("2", true); err != nil {
		t.Fatal(err)
	}
	if got := boards(); !reflect.DeepEqual(got, []string{"movie", "stock"}) {
		t.Errorf("boards = %v", got)
	}
	if got := subscribers("joke"); len(got) != 0 {
		t.Errorf("joke subscribers = %v", got)
	}

	// removed member keeps subscriptions but is not checked
	if err := Update("2", false); err != nil {
		t.Fatal(err)
	}
	if got := boards(); !reflect.DeepEqual(got, []string{"stock"}) {
		t.Errorf("boards = %v", got)
	}
	if got := subscribers("stock"); !reflect.DeepEqual(got, []string{"1"}) {
		t.Errorf("stock subscribers = %v", got)
	}

	// disabled user (blocked the bot) is not checked
	if err := user.Disable("1"); err != nil {
		t.Fatal(err)
	}
	if err := Update("1", true); err != nil {
		t.Fatal(err)
	}
	if got := boards(); len(got) != 0 {
		t.Errorf("boards = %v", got)
	}
}
