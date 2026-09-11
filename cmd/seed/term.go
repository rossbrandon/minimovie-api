package main

import (
	"fmt"
	"os"
	"sync"
	"time"
)

const (
	statusMaxWidth    = 120
	plainStatusPeriod = 10 * time.Second
)

type terminal struct {
	mu        sync.Mutex
	out       *os.File
	isTTY     bool
	status    string
	lastPlain time.Time
}

func (t *terminal) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.isTTY && t.status != "" {
		fmt.Fprint(t.out, "\r\033[K")
	}
	n, err := t.out.Write(p)
	if t.isTTY && t.status != "" {
		t.redraw()
	}
	return n, err
}

func newTerminal(out *os.File) *terminal {
	t := &terminal{out: out}
	if fi, err := out.Stat(); err == nil {
		t.isTTY = fi.Mode()&os.ModeCharDevice != 0
	}
	return t
}

func (t *terminal) setStatus(s string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if len(s) > statusMaxWidth {
		s = s[:statusMaxWidth]
	}
	t.status = s
	if t.isTTY {
		t.redraw()
		return
	}
	if time.Since(t.lastPlain) >= plainStatusPeriod {
		fmt.Fprintln(t.out, s)
		t.lastPlain = time.Now()
	}
}

func (t *terminal) finish() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.status == "" {
		return
	}
	if t.isTTY {
		t.redraw()
		fmt.Fprintln(t.out)
	} else {
		fmt.Fprintln(t.out, t.status)
	}
	t.status = ""
}

func (t *terminal) redraw() {
	fmt.Fprintf(t.out, "\r\033[K%s", t.status)
}
