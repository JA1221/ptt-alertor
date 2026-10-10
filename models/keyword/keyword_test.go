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

func subscribe(t *testing.T, account, boardName string, keywords ...string) {
	t.Helper()
	if err := user.Register(account, 0); err != nil {
		t.Fatal(err)
	}
	err := user.Update(account, func(u *user.User) error {
		u.Subscribes.AddKeywords(boardName, keywords)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestSync(t *testing.T) {
	redistest.Flush()
	subscribe(t, "1", "stock", "a")
	subscribe(t, "2", "Stock", "b")
	subscribe(t, "2", "joke", "c")
	if err := board.Add("old"); err != nil {
		t.Fatal(err)
	}
	if err := board.SaveArticles("old", article.Articles{{ID: 1}}); err != nil {
		t.Fatal(err)
	}

	if err := Sync([]string{"1", "2", "404"}); err != nil {
		t.Fatal(err)
	}
	subs, _ := Subscribers("STOCK")
	sort.Strings(subs)
	if !reflect.DeepEqual(subs, []string{"1", "2"}) {
		t.Errorf("stock subscribers = %v", subs)
	}
	boards := board.List()
	sort.Strings(boards)
	if !reflect.DeepEqual(boards, []string{"joke", "stock"}) {
		t.Errorf("boards = %v", boards)
	}
	if saved, _ := board.SavedArticles("old"); saved != nil {
		t.Error("saved articles of unsubscribed board should be removed")
	}

	// last keyword of joke removed
	err := user.Update("2", func(u *user.User) error {
		u.Subscribes.RemoveKeywords("joke", []string{"*"})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := Sync([]string{"1"}); err != nil {
		t.Fatal(err)
	}
	if subs, _ := Subscribers("joke"); len(subs) != 0 {
		t.Errorf("joke subscribers = %v", subs)
	}
	if subs, _ := Subscribers("stock"); !reflect.DeepEqual(subs, []string{"1"}) {
		t.Errorf("accounts not allowed should be dropped, stock subscribers = %v", subs)
	}
	if boards := board.List(); !reflect.DeepEqual(boards, []string{"stock"}) {
		t.Errorf("boards = %v", boards)
	}
}
