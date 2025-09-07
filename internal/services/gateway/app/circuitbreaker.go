package app

import (
	"sync"
	"time"
)

// Semplice Circuit Breaker senza dipendenze esterne.
// Stati: Closed -> (troppi errori) -> Open -> (dopo openFor) -> HalfOpen
// In HalfOpen consentiamo 1 prova: ok => Closed, ko => Open di nuovo.

type breakerState int

const (
	stateClosed breakerState = iota
	stateOpen
	stateHalfOpen
)

type CircuitBreaker struct {
	mu         sync.Mutex
	state      breakerState
	failures   int
	failThresh int
	openUntil  time.Time
	openFor    time.Duration
}

func NewCircuitBreaker(failuresThreshold int, openFor time.Duration) *CircuitBreaker {
	if failuresThreshold < 1 {
		failuresThreshold = 1
	}
	if openFor <= 0 {
		openFor = 10 * time.Second
	}
	return &CircuitBreaker{
		state:      stateClosed,
		failThresh: failuresThreshold,
		openFor:    openFor,
	}
}

func (cb *CircuitBreaker) Allow() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	now := time.Now()
	switch cb.state {
	case stateClosed:
		return true
	case stateOpen:
		if now.After(cb.openUntil) {
			cb.state = stateHalfOpen
			return true
		}
		return false
	case stateHalfOpen:
		// Consenti una sola richiesta di prova
		// (la prossima decisione verrà presa da Success/Fail)
		// Blocchiamo altre concorrenze nello stesso istante.
		return true
	default:
		return true
	}
}

func (cb *CircuitBreaker) Success() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.failures = 0
	cb.state = stateClosed
}

func (cb *CircuitBreaker) Fail() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case stateClosed:
		cb.failures++
		if cb.failures >= cb.failThresh {
			cb.tripOpen()
		}
	case stateHalfOpen:
		cb.tripOpen()
	default:
		// already open; noop
	}
}

func (cb *CircuitBreaker) tripOpen() {
	cb.state = stateOpen
	cb.openUntil = time.Now().Add(cb.openFor)
	cb.failures = 0
}
