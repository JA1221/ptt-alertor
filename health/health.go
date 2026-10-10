// Package health records results of crawling and messaging,
// decides when to alert owners and reports status for the status page and commands.
package health

import (
	"fmt"
	"sort"
	"sync"
	"time"
)

const (
	// BoardAlertAfter alerts when a board keeps failing this long
	BoardAlertAfter = 10 * time.Minute
	// PTTAlertFailures alerts when PTT monitor fails this many times in a row (once a minute)
	PTTAlertFailures = 3
	// boardProblemAfter marks status unhealthy when a board keeps failing this long
	boardProblemAfter = 2 * time.Minute
	// stalledAfter marks status unhealthy when checking stops
	stalledAfter = 5 * time.Minute
)

type boardState struct {
	lastSuccess  time.Time
	lastAttempt  time.Time
	lastError    string
	failingSince time.Time
	alerted      bool
}

type pttState struct {
	lastCheck time.Time
	failures  int
	lastError string
	alerted   bool
}

type telegramState struct {
	lastSent      time.Time
	lastSendError string
	lastSendErrAt time.Time
	lastPoll      time.Time
	lastPollError string
	lastPollErrAt time.Time
}

type commentState struct {
	tracked   int
	lastCheck time.Time
	lastError string
}

// Tracker keeps health states in memory, it is safe for concurrent use
type Tracker struct {
	mu       sync.Mutex
	now      func() time.Time
	started  time.Time
	boards   map[string]*boardState
	ptt      pttState
	telegram telegramState
	comments commentState
}

// New creates a Tracker
func New() *Tracker {
	return newWithClock(time.Now)
}

func newWithClock(now func() time.Time) *Tracker {
	return &Tracker{
		now:     now,
		started: now(),
		boards:  make(map[string]*boardState),
	}
}

func (t *Tracker) board(name string) *boardState {
	b, ok := t.boards[name]
	if !ok {
		b = &boardState{}
		t.boards[name] = b
	}
	return b
}

// SetBoards keeps states of checking boards only
func (t *Tracker) SetBoards(names []string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	keep := make(map[string]bool, len(names))
	for _, name := range names {
		keep[name] = true
		t.board(name)
	}
	for name := range t.boards {
		if !keep[name] {
			delete(t.boards, name)
		}
	}
}

// BoardChecked records result of fetching a board
func (t *Tracker) BoardChecked(name string, err error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	b := t.board(name)
	now := t.now()
	b.lastAttempt = now
	if err == nil {
		b.lastSuccess = now
		b.lastError = ""
		b.failingSince = time.Time{}
		return
	}
	b.lastError = err.Error()
	if b.failingSince.IsZero() {
		b.failingSince = now
	}
}

// PTTChecked records result of PTT monitor
func (t *Tracker) PTTChecked(err error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.ptt.lastCheck = t.now()
	if err == nil {
		t.ptt.failures = 0
		t.ptt.lastError = ""
		return
	}
	t.ptt.failures++
	t.ptt.lastError = err.Error()
}

// TelegramSent records result of sending a message
func (t *Tracker) TelegramSent(err error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if err == nil {
		t.telegram.lastSent = t.now()
		t.telegram.lastSendError = ""
		return
	}
	t.telegram.lastSendError = err.Error()
	t.telegram.lastSendErrAt = t.now()
}

// TelegramPolled records result of receiving updates
func (t *Tracker) TelegramPolled(err error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if err == nil {
		t.telegram.lastPoll = t.now()
		t.telegram.lastPollError = ""
		return
	}
	t.telegram.lastPollError = err.Error()
	t.telegram.lastPollErrAt = t.now()
}

// CommentsChecked records a round of comment checking
func (t *Tracker) CommentsChecked(tracked int, err error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.comments.tracked = tracked
	t.comments.lastCheck = t.now()
	t.comments.lastError = ""
	if err != nil {
		t.comments.lastError = err.Error()
	}
}

func (t *Tracker) pttDown() bool {
	return t.ptt.failures >= PTTAlertFailures
}

// Evaluate returns alert and recovery messages for state changes since last call
func (t *Tracker) Evaluate() (msgs []string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	now := t.now()

	if t.pttDown() && !t.ptt.alerted {
		t.ptt.alerted = true
		msgs = append(msgs, fmt.Sprintf("⚠️ PTT 無法連線（已連續 %d 分鐘）：%s", t.ptt.failures, t.ptt.lastError))
	}
	if t.ptt.failures == 0 && t.ptt.alerted {
		t.ptt.alerted = false
		msgs = append(msgs, "✅ PTT 已恢復連線")
	}

	for _, name := range t.boardNames() {
		b := t.boards[name]
		failing := !b.failingSince.IsZero()
		if failing && !b.alerted && !t.pttDown() && now.Sub(b.failingSince) >= BoardAlertAfter {
			b.alerted = true
			msgs = append(msgs, fmt.Sprintf("⚠️ %s 已連續 %d 分鐘抓取失敗：%s", name, int(now.Sub(b.failingSince).Minutes()), b.lastError))
		}
		if !failing && b.alerted {
			b.alerted = false
			msgs = append(msgs, fmt.Sprintf("✅ %s 已恢復正常", name))
		}
	}
	return msgs
}

func (t *Tracker) boardNames() []string {
	names := make([]string, 0, len(t.boards))
	for name := range t.boards {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// BoardStatus is status of a checking board
type BoardStatus struct {
	Name         string
	LastSuccess  time.Time
	LastError    string
	FailingSince time.Time
}

// OK reports the board is not failing
func (b BoardStatus) OK() bool {
	return b.FailingSince.IsZero()
}

// Snapshot is a copy of all statuses
type Snapshot struct {
	Now      time.Time
	Started  time.Time
	Problems []string

	PTTLastCheck time.Time
	PTTError     string

	TelegramLastSent  time.Time
	TelegramSendError string
	TelegramLastPoll  time.Time
	TelegramPollError string

	Boards []BoardStatus

	CommentsTracked   int
	CommentsLastCheck time.Time
	CommentsError     string
}

// Healthy reports no problem found
func (s Snapshot) Healthy() bool {
	return len(s.Problems) == 0
}

// Snapshot returns current statuses and problems
func (t *Tracker) Snapshot() Snapshot {
	t.mu.Lock()
	defer t.mu.Unlock()
	now := t.now()
	s := Snapshot{
		Now:               now,
		Started:           t.started,
		PTTLastCheck:      t.ptt.lastCheck,
		PTTError:          t.ptt.lastError,
		TelegramLastSent:  t.telegram.lastSent,
		TelegramSendError: t.telegram.lastSendError,
		TelegramLastPoll:  t.telegram.lastPoll,
		TelegramPollError: t.telegram.lastPollError,
		CommentsTracked:   t.comments.tracked,
		CommentsLastCheck: t.comments.lastCheck,
		CommentsError:     t.comments.lastError,
	}

	if t.ptt.failures > 0 {
		s.Problems = append(s.Problems, "PTT 無法連線："+t.ptt.lastError)
	}
	if t.telegram.lastPollError != "" {
		s.Problems = append(s.Problems, "Telegram 接收訊息失敗："+t.telegram.lastPollError)
	}
	if t.telegram.lastSendError != "" {
		s.Problems = append(s.Problems, "Telegram 送出訊息失敗："+t.telegram.lastSendError)
	}
	// time since started is used before the first round finishes
	since := func(tm time.Time) time.Duration {
		if tm.IsZero() {
			tm = t.started
		}
		return now.Sub(tm)
	}
	if !t.comments.lastCheck.IsZero() || t.comments.tracked > 0 {
		if since(t.comments.lastCheck) > stalledAfter {
			s.Problems = append(s.Problems, "推文追蹤停止檢查")
		}
	}
	for _, name := range t.boardNames() {
		b := t.boards[name]
		s.Boards = append(s.Boards, BoardStatus{
			Name:         name,
			LastSuccess:  b.lastSuccess,
			LastError:    b.lastError,
			FailingSince: b.failingSince,
		})
		switch {
		case !b.failingSince.IsZero() && now.Sub(b.failingSince) >= boardProblemAfter:
			s.Problems = append(s.Problems, name+" 抓取失敗："+b.lastError)
		case since(b.lastAttempt) > stalledAfter:
			s.Problems = append(s.Problems, name+" 停止檢查")
		}
	}
	return s
}
