package main

import (
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

var spinnerFrames = []rune("⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏")

// progress reports how a sync is going. On a terminal it shows a spinner with
// a running count of synced issues; otherwise it prints plain lines, and only
// when the sync starts or pauses for a rate limit.
type progress struct {
	out    io.Writer
	tty    bool
	status string // what is being synced

	mu      sync.Mutex
	synced  int
	waiting string // why the sync is paused, if it is

	stop chan struct{}
	done chan struct{}
}

func startProgress(out io.Writer, tty bool, status string) *progress {
	p := &progress{out: out, tty: tty, status: status}
	if !tty {
		fmt.Fprintln(out, status)
		return p
	}
	p.stop = make(chan struct{})
	p.done = make(chan struct{})
	go p.spin()
	return p
}

func (p *progress) spin() {
	defer close(p.done)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for frame := 0; ; frame++ {
		p.draw(spinnerFrames[frame%len(spinnerFrames)])
		select {
		case <-ticker.C:
		case <-p.stop:
			fmt.Fprint(p.out, "\r\033[K") // clear the status line
			return
		}
	}
}

func (p *progress) draw(frame rune) {
	p.mu.Lock()
	line := fmt.Sprintf("%c %s: %d synced", frame, p.status, p.synced)
	if p.waiting != "" {
		line += " (" + p.waiting + ")"
	}
	p.mu.Unlock()
	fmt.Fprint(p.out, "\r\033[K"+line)
}

func (p *progress) issueSynced() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.synced++
}

// setWaiting says why the sync is paused. An empty reason means it resumed.
func (p *progress) setWaiting(reason string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.waiting = reason
	if !p.tty && reason != "" {
		fmt.Fprintln(p.out, reason)
	}
}

// finish stops the spinner and clears its line.
func (p *progress) finish() {
	if p.tty {
		close(p.stop)
		<-p.done
	}
}

func isTerminal(f *os.File) bool {
	info, err := f.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}
