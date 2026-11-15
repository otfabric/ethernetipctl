// SPDX-License-Identifier: MIT

package adapter

import (
	"strconv"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/otfabric/ethernetipctl/internal/types"
)

// stalledLog is a log that holds its first event until release is closed, and records
// every event it gets.
type stalledLog struct {
	release chan struct{}
	mu      sync.Mutex
	got     []types.AdapterEvent
}

func (l *stalledLog) log(ev types.AdapterEvent) {
	l.mu.Lock()
	first := len(l.got) == 0
	l.got = append(l.got, ev)
	l.mu.Unlock()
	if first {
		<-l.release
	}
}

func (l *stalledLog) events() []types.AdapterEvent {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]types.AdapterEvent(nil), l.got...)
}

func event(i int) types.AdapterEvent { return types.AdapterEvent{Event: "e" + strconv.Itoa(i)} }

// While log does not return, emit does not wait: up to eventQueue events wait, the rest
// are counted, and a "dropped" event with their number takes their place in the order.
func TestEmitterDropsWhileLogStalls(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		l := &stalledLog{release: make(chan struct{})}
		e := newEmitter(l.log)
		e.emit(event(0))
		synctest.Wait() // log holds e0
		for i := 1; i <= eventQueue+10; i++ {
			e.emit(event(i)) // never waits
		}
		close(l.release)
		synctest.Wait() // e1 to e1024 written
		e.emit(event(-1))
		e.close(time.Second)

		got := l.events()
		if len(got) != eventQueue+3 {
			t.Fatalf("%d events, want %d", len(got), eventQueue+3)
		}
		for i := 0; i <= eventQueue; i++ {
			if got[i].Event != event(i).Event {
				t.Fatalf("event %d is %s", i, got[i].Event)
			}
		}
		if d := got[eventQueue+1]; d.Event != "dropped" || d.Detail != "10 event(s) not written: the output did not keep up" || d.Timestamp == "" {
			t.Errorf("dropped event %+v", d)
		}
		if got[eventQueue+2].Event != "e-1" {
			t.Errorf("last event %+v", got[eventQueue+2])
		}
	})
}

// close waits for log at most as long as it is told; the events dropped by then are
// counted in a last "dropped" event once log returns, and emit after close does nothing.
func TestEmitterCloseDoesNotWaitForAStalledLog(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		l := &stalledLog{release: make(chan struct{})}
		e := newEmitter(l.log)
		e.emit(event(0))
		synctest.Wait()
		for i := 1; i <= eventQueue+5; i++ {
			e.emit(event(i))
		}
		start := time.Now()
		e.close(drainTimeout)
		if d := time.Since(start); d != drainTimeout {
			t.Errorf("close took %v, want %v", d, drainTimeout)
		}
		e.emit(event(-1))
		e.close(drainTimeout) // twice: nothing more
		close(l.release)
		synctest.Wait()

		got := l.events()
		if len(got) != eventQueue+2 {
			t.Fatalf("%d events, want %d", len(got), eventQueue+2)
		}
		if d := got[len(got)-1]; d.Event != "dropped" || d.Detail != "5 event(s) not written: the output did not keep up" {
			t.Errorf("last event %+v", d)
		}
	})
}
