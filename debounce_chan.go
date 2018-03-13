package gsync

import (
	"time"
)

type debounceChan struct {
	input         chan interface{}
	output        chan interface{}
	minimalPass   time.Duration
	lastEventTime time.Time
}

func DebouncePipeChan(minPass time.Duration) (in chan interface{}, out chan interface{}) {
	dc := &debounceChan{
		input:       make(chan interface{}),
		output:      make(chan interface{}, 100),
		minimalPass: minPass,
	}

	go func() {
		for {
			select {
			case v, ok := <-dc.input:
				if !ok {
					close(dc.output)
					return
				}
				if time.Now().Sub(dc.lastEventTime) > dc.minimalPass {
					dc.output <- v
					dc.lastEventTime = time.Now()
				}
			}
		}
	}()

	return dc.input, dc.output
}
