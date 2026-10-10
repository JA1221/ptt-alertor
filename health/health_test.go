package health

import (
	"errors"
	"strings"
	"testing"
	"time"
)

type clock struct{ t time.Time }

func (c *clock) now() time.Time          { return c.t }
func (c *clock) advance(d time.Duration) { c.t = c.t.Add(d) }
func newTracker() (*Tracker, *clock) {
	c := &clock{time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC)}
	tr := newWithClock(c.now)
	tr.SetBot("bot")
	return tr, c
}
func contains(msgs []string, s string) bool { return strings.Contains(strings.Join(msgs, "\n"), s) }

var errFetch = errors.New("timeout")

func TestBoardAlertOnceAndRecovery(t *testing.T) {
	tr, c := newTracker()
	tr.SetBoards([]string{"stock"})
	tr.BoardChecked("stock", nil)

	tr.BoardChecked("stock", errFetch)
	c.advance(BoardAlertAfter - time.Second)
	tr.BoardChecked("stock", errFetch)
	if msgs := tr.Evaluate(); len(msgs) != 0 {
		t.Fatalf("alert too early: %v", msgs)
	}

	c.advance(time.Second)
	tr.BoardChecked("stock", errFetch)
	if msgs := tr.Evaluate(); len(msgs) != 1 || !contains(msgs, "stock 已連續 10 分鐘抓取失敗：timeout") {
		t.Fatalf("Evaluate() = %v", msgs)
	}
	c.advance(time.Minute)
	if msgs := tr.Evaluate(); len(msgs) != 0 {
		t.Fatalf("alert repeated: %v", msgs)
	}

	tr.BoardChecked("stock", nil)
	if msgs := tr.Evaluate(); len(msgs) != 1 || !contains(msgs, "✅ stock 已恢復正常") {
		t.Fatalf("Evaluate() = %v", msgs)
	}
	if msgs := tr.Evaluate(); len(msgs) != 0 {
		t.Fatalf("recovery repeated: %v", msgs)
	}
}

func TestPTTDownSuppressesBoardAlerts(t *testing.T) {
	tr, c := newTracker()
	tr.SetBoards([]string{"stock"})
	tr.BoardChecked("stock", errFetch)
	for i := 0; i < AlertFailures; i++ {
		c.advance(5 * time.Minute)
		tr.PTTChecked(errFetch)
	}
	msgs := tr.Evaluate()
	if len(msgs) != 1 || !contains(msgs, "PTT 無法連線") {
		t.Fatalf("Evaluate() = %v", msgs)
	}

	tr.PTTChecked(nil)
	msgs = tr.Evaluate()
	if !contains(msgs, "PTT 已恢復連線") {
		t.Fatalf("Evaluate() = %v", msgs)
	}
	// board keeps failing after PTT is back
	if !contains(msgs, "stock 已連續") {
		t.Errorf("board alert should follow PTT recovery: %v", msgs)
	}
}

func TestSnapshotProblems(t *testing.T) {
	tr, c := newTracker()
	tr.SetBoards([]string{"joke", "stock"})
	tr.BoardChecked("joke", nil)
	tr.BoardChecked("stock", nil)
	tr.TelegramPolled(nil)
	if s := tr.Snapshot(); !s.Healthy() || len(s.Boards) != 2 {
		t.Fatalf("Snapshot() = %+v", s)
	}

	tr.BoardChecked("stock", errFetch)
	c.advance(3 * time.Minute)
	tr.BoardChecked("joke", nil)
	tr.BoardChecked("stock", errFetch)
	s := tr.Snapshot()
	if s.Healthy() || !contains(s.Problems, "stock 抓取失敗") || contains(s.Problems, "joke") {
		t.Fatalf("Problems = %v", s.Problems)
	}

	// checking stopped
	c.advance(stalledAfter + time.Second)
	if s := tr.Snapshot(); !contains(s.Problems, "joke 停止檢查") {
		t.Fatalf("Problems = %v", s.Problems)
	}

	tr.SetBoards([]string{"joke"})
	if s := tr.Snapshot(); len(s.Boards) != 1 || s.Boards[0].Name != "joke" {
		t.Errorf("Boards = %v", s.Boards)
	}
}

func TestSnapshotTelegramProblems(t *testing.T) {
	tr, _ := newTracker()
	tr.TelegramSent(errors.New("Forbidden"))
	if s := tr.Snapshot(); !contains(s.Problems, "Telegram 送出訊息失敗：Forbidden") {
		t.Fatalf("Problems = %v", s.Problems)
	}
	tr.TelegramSent(nil)
	if s := tr.Snapshot(); !s.Healthy() {
		t.Fatalf("Problems = %v", s.Problems)
	}
}

func TestSnapshotTelegramNotConnected(t *testing.T) {
	tr := New()
	if s := tr.Snapshot(); !contains(s.Problems, "Telegram 尚未連線") {
		t.Fatalf("Problems = %v", s.Problems)
	}
}

func TestRedisAlert(t *testing.T) {
	tr, _ := newTracker()
	for i := 0; i < AlertFailures; i++ {
		tr.RedisChecked(errFetch)
	}
	if msgs := tr.Evaluate(); len(msgs) != 1 || !contains(msgs, "Redis 無法連線") {
		t.Fatalf("Evaluate() = %v", msgs)
	}
	tr.RedisChecked(nil)
	if msgs := tr.Evaluate(); len(msgs) != 1 || !contains(msgs, "Redis 已恢復") {
		t.Fatalf("Evaluate() = %v", msgs)
	}
}

func TestAgo(t *testing.T) {
	now := time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)
	tests := map[time.Time]string{
		{}:                        "從未",
		now.Add(-5 * time.Second): "5 秒前",
		now.Add(-3 * time.Minute): "3 分鐘前",
		now.Add(-2 * time.Hour):   "2 小時前",
		now.Add(-50 * time.Hour):  "2 天前",
	}
	for tm, want := range tests {
		if got := Ago(now, tm); got != want {
			t.Errorf("Ago(%v) = %q, want %q", tm, got, want)
		}
	}
}
