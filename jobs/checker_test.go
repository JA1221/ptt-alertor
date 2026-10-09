package jobs

import (
	"testing"

	"github.com/Ptt-Alertor/ptt-alertor/models/board"
)

func TestCheckNewArticleSkipsBoardBeingChecked(t *testing.T) {
	checkingBoards.Store("test", true)
	defer checkingBoards.Delete("test")

	bd := board.NewBoard(nil, nil)
	bd.Name = "test"
	// would panic on nil driver if the check was not skipped
	checkNewArticle(bd, make(chan *board.Board, 1))

	if _, ok := checkingBoards.Load("test"); !ok {
		t.Error("skipped check must not release the board lock held by another check")
	}
}
