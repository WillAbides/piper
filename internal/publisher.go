package internal

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"sync"
	"time"
)

// EventSink a sink for events
type EventSink interface {
	FlushEvents(ctx context.Context, cache [][]byte) error
}

// Publisher publishes events
type Publisher struct {
	MaxQueueSize  int
	Sink          EventSink
	FlushInterval time.Duration

	mutex   sync.Mutex
	cache   [][]byte
	running bool
}

// Run runs Publisher
func (p *Publisher) Run(ctx context.Context, scanner *bufio.Scanner) error {
	var flushInterval time.Duration

	p.mutex.Lock()
	if p.running {
		p.mutex.Unlock()
		return fmt.Errorf("already running")
	}
	p.running = true
	flushInterval = p.FlushInterval
	p.mutex.Unlock()
	defer func() {
		p.mutex.Lock()
		p.running = false
		p.mutex.Unlock()
	}()

	flushErrMutex := new(sync.Mutex)
	var flushErr error

	getFlushErr := func() error {
		flushErrMutex.Lock()
		got := flushErr
		flushErrMutex.Unlock()
		return got
	}

	resetTicker := func() {}

	if flushInterval != 0 {
		ticker := time.NewTicker(flushInterval)
		resetTicker = func() {
			ticker.Reset(flushInterval)
		}
		go func() {
			for range ticker.C {
				err2 := p.flush(ctx)
				if err2 != nil {
					flushErrMutex.Lock()
					flushErr = err2
					flushErrMutex.Unlock()
					return
				}
			}
		}()
	}

	var err error
	for scanner.Scan() {
		b := scanner.Bytes()
		b = bytes.TrimSpace(b)
		if len(b) == 0 {
			continue
		}
		err = p.addEvent(ctx, resetTicker, scanner.Bytes())
		if err != nil {
			return err
		}
		if getFlushErr() != nil {
			return getFlushErr()
		}
		if ctx.Err() != nil {
			break
		}
	}

	err = p.flush(ctx)
	if err != nil {
		return err
	}
	return scanner.Err()
}

func (p *Publisher) addEvent(ctx context.Context, resetTicker func(), data []byte) error {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	p.cache = append(p.cache, data)
	if len(p.cache) == 1 {
		resetTicker()
	}
	if len(p.cache) == 0 || len(p.cache) < p.MaxQueueSize {
		return nil
	}
	err := p.Sink.FlushEvents(ctx, p.cache)
	if err != nil {
		return err
	}
	p.cache = p.cache[:0]
	return nil
}

func (p *Publisher) flush(ctx context.Context) error {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	return p.Sink.FlushEvents(ctx, p.cache)
}
