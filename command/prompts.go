package command

import (
	"sync"
	"time"
)

// promptTTL is how long the bot waits for input asked by a menu
const promptTTL = 10 * time.Minute

// prompt is input the bot waits for after a menu button,
// board name when board is empty, otherwise keywords of board
type prompt struct {
	board string
	at    time.Time
}

// prompts keeps one waiting input per account in memory
type prompts struct {
	mu sync.Mutex
	m  map[string]prompt
}

func (p *prompts) set(account, board string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.m == nil {
		p.m = make(map[string]prompt)
	}
	p.m[account] = prompt{board: board, at: time.Now()}
}

// take removes and returns waiting input of account
func (p *prompts) take(account string) (prompt, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	pr, ok := p.m[account]
	delete(p.m, account)
	return pr, ok && time.Since(pr.at) < promptTTL
}
