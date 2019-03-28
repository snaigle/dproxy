package main

import (
	"sync"
)

// ControlRegistry maps a client ID to Control structures
type ControlRegistry struct {
	controls map[string]*Control
	sync.RWMutex
}

func NewControlRegistry() *ControlRegistry {
	return &ControlRegistry{
		controls: make(map[string]*Control),
	}
}

func (r *ControlRegistry) Get(clientId string) *Control {
	r.RLock()
	defer r.RUnlock()
	return r.controls[clientId]
}

func (r *ControlRegistry) Add(clientId string, ctl *Control) (oldCtl *Control) {
	r.Lock()
	defer r.Unlock()

	oldCtl = r.controls[clientId]

	r.controls[clientId] = ctl
	return
}

func (r *ControlRegistry) Foreach(f func(*Control) bool) {
	r.RLock()
	defer r.RUnlock()
	for _, v := range r.controls {
		if f(v) {
			break
		}
	}

}

func (r *ControlRegistry) Del(clientId string) {
	r.Lock()
	defer r.Unlock()
	if r.controls[clientId] != nil {
		delete(r.controls, clientId)
	}
}
