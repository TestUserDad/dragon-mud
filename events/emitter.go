// Copyright (c) 2016-2017 Brandon Buck

package events

import (
	"errors"
	"strings"
	"sync"

	"github.com/bbuck/dragon-mud/logger"
)

// ErrHalt is a simple error used in place of just halting execution. Returning
// an error from a handlers Call will halt event execution, which may happen
// if a real error happens, or perhaps for some reason you just want to stop
// the event trigger. Therefore this error represents no particular error has
// ocurred but the event execution should be halted.
var ErrHalt = errors.New("intentional halt of event execution")

// Data is a generic map from strings to any values that can be used as a means
// to wrap a chunk of dynamic data and pass them to event handlers.
// Event data should contain data specific to the event being fired that would
// allow handlers to make actionable response to. Such as an "damage_taken"
// event might have a map containing "source" (who did the damage), "target"
// (who received the damage), and then data about the damage itself.
type Data map[string]interface{}

// NewData returns an empty map[string]interface{} wrapped in the Data type,
// as an easy way to seen event emissions with empty data (where nil would mean
// no data).
func NewData() Data {
	return Data(make(map[string]interface{}))
}

// Handler is a type with a Call function that accepts Data, and represents some
// callable type that wants to perform some action when an event is emitted.
type Handler interface {
	Call(Data) error
}

// HandlerFunc wraps a Go func in a painless way to match the events.Handler
// interface.
type HandlerFunc func(Data) error

// Call will just call the funtion the HandlerFunc type is wrapping and return
// it's results. This allows functions to fit the events.Handler interface
// painlessly.
func (hf HandlerFunc) Call(d Data) error {
	return hf(d)
}

// Emitter represents a type capable of handling a list of callable actions to
// act on event data.
type Emitter struct {
	handlers         map[string]*handlers
	mutex            *sync.RWMutex
	log              logger.Log
	oneTimeEmissions map[string]Data
}

// NewEmitter generates a new event emitter with the given name used for logging
// purposes.
func NewEmitter(l logger.Log) *Emitter {
	return &Emitter{
		handlers:         make(map[string]*handlers),
		mutex:            new(sync.RWMutex),
		log:              l,
		oneTimeEmissions: make(map[string]Data),
	}
}

// On registers the handler for the given event.
// Events registered in this manner will be called every time this event is
// emitted.
func (e *Emitter) On(evt string, h Handler) {
	var (
		hs *handlers
		ok bool
	)

	e.mutex.RLock()
	if hs, ok = e.handlers[evt]; ok {
		e.mutex.RUnlock()
	} else {
		e.mutex.RUnlock()
		e.mutex.Lock()
		hs = newHandlers()
		e.handlers[evt] = hs
		e.mutex.Unlock()
	}
	hs.add(h)

	e.mutex.RLock()
	defer e.mutex.RUnlock()
	if data, ok := e.oneTimeEmissions[evt]; ok {
		h.Call(copyData(data))
	}
}

// Once resgisters a handler for an event that will fire one time and then
// drop from the handler list.
// This is great for one time handlers, things that don't need to happen
// everytime the event is emitted.
func (e *Emitter) Once(evt string, h Handler) {
	e.mutex.RLock()
	if data, ok := e.oneTimeEmissions[evt]; ok {
		h.Call(copyData(data))
		e.mutex.RUnlock()

		return
	}

	var (
		hs *handlers
		ok bool
	)
	if hs, ok = e.handlers[evt]; ok {
		e.mutex.RUnlock()
	} else {
		e.mutex.RUnlock()
		e.mutex.Lock()
		hs = newHandlers()
		e.handlers[evt] = hs
		e.mutex.Unlock()
	}
	hs.addOnce(h)
}

// Off will remove all handlers for the given event, including it's before and
// after handlers.
func (e *Emitter) Off(evt string) {
	e.off("before:" + evt)
	e.off(evt)
	e.off("after:" + evt)
}

// clear handlers for event
func (e *Emitter) off(evt string) {
	e.mutex.RLock()
	defer e.mutex.RUnlock()
	if hs, ok := e.handlers[evt]; ok {
		hs.clear()
	}
}

// Emit will call all handlers and once handlers assigned to listen to the event
// as well as emitting a before:<event> and after:<event> before and after.
// This method is asyncronous and returns no values directly, failures get
// logged to the log target(s). Returns a readonly channel of struct{} (emtpy
// data) That is written two (once) when the emission has completed.
func (e *Emitter) Emit(evt string, d Data) <-chan struct{} {
	if strings.HasPrefix(evt, "before:") || strings.HasPrefix(evt, "after:") {
		if e.log != nil {
			e.log.WithFields(logger.Fields{
				"event": evt,
				"data":  d,
			}).Warn("Cannot emit meta events 'before' or 'after' directly.")
		}
	}

	if d == nil {
		d = NewData()
	} else {
		d = copyData(d)
	}

	done := make(chan struct{}, 1)
	go func() {
		err := e.emit("before:"+evt, d)
		if err == nil {
			err = e.emit(evt, d)
		}
		if err == nil {
			err = e.emit("after:"+evt, d)
		}

		if err != nil {
			if err == ErrHalt {
				if e.log != nil {
					e.log.WithFields(logger.Fields{
						"event": evt,
						"data":  d,
					}).Debug("Event emission halted.")
				}
			} else {
				if e.log != nil {
					e.log.WithFields(logger.Fields{
						"error": err.Error(),
						"event": evt,
						"data":  d,
					}).Error("Failed during execution of event handlers.")
				}
			}
		}

		done <- struct{}{}
	}()

	return done
}

// EmitOnce is similar to emit except it's designed to handle events intended
// that are only intended to be fired one time during the lifetime of the
// application. Any new handlers that are added for the one time emission are
// immediatley triggered with the data from the `EmitOnce` call.
func (e *Emitter) EmitOnce(evt string, d Data) <-chan struct{} {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	d = copyData(d)
	e.oneTimeEmissions["before:"+evt] = d
	e.oneTimeEmissions[evt] = d
	e.oneTimeEmissions["after:"+evt] = d

	done := e.Emit(evt, d)

	return done
}

// this handles the meat of emitting events, it will iterate over the one time
// handlers and clear out all (or only those that get touched) and then all
// persistent handlers
func (e *Emitter) emit(evt string, d Data) error {
	e.mutex.RLock()
	defer e.mutex.RUnlock()
	if hs, ok := e.handlers[evt]; ok {
		return hs.call(d)
	}

	return nil
}

func copyData(d Data) Data {
	nd := make(Data)
	for k, v := range d {
		switch t := v.(type) {
		case Data:
			nd[k] = copyData(d)
		case map[string]interface{}:
			nd[k] = copyData(Data(t))
		default:
			nd[k] = v
		}
	}

	return nd
}
