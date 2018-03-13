package main

import (
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

type notifyRecords struct {
	name          string
	lastEventTime time.Time
	event         *fsnotify.Event
}

type debouncePipe struct {
	sync.Mutex
	records     map[string]*notifyRecords
	minimalPass time.Duration
	input       chan fsnotify.Event
	output      chan fsnotify.Event
}

var dc *debouncePipe

func NotifyPipeChan(minPass time.Duration) (in chan fsnotify.Event, out chan fsnotify.Event) {
	dc := &debouncePipe{
		records:     make(map[string]*notifyRecords),
		input:       make(chan fsnotify.Event),
		output:      make(chan fsnotify.Event, 100),
		minimalPass: minPass,
	}
	go func() {
		ticker := time.NewTicker(minPass)
		defer ticker.Stop()
		for {
			select {
			case e, ok := <-dc.input:
				if !ok {
					close(dc.output)
					return
				}

				now := time.Now()
				dc.Lock()
				if (e.Op & fsnotify.Write) == fsnotify.Write {
					//multiple write events will be sent on a save operation.
					//so just mark event received time and notify later after minPass past
					if i, ok := dc.records[e.Name]; ok {
						i.lastEventTime = now
						i.event = &e
					} else {
						dc.records[e.Name] = &notifyRecords{
							name:          e.Name,
							lastEventTime: now,
							event:         &e,
						}
					}
					dc.Unlock()
					continue
				}

				dc.output <- e
				dc.Unlock()
			case <-ticker.C:
				dc.Lock()
				now := time.Now()
				for _, r := range dc.records {
					if r.event != nil && now.Sub(r.lastEventTime) > minPass {
						dc.output <- *r.event
						r.event = nil
					}
				}
				dc.Unlock()
			}
		}
	}()

	return dc.input, dc.output
}
