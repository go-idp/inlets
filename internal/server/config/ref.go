package config

import (
	"sync"
)

// Ref holds the live server config document for hot reload.
type Ref struct {
	mu     sync.RWMutex
	config *FileConfig
}

func NewRef(initial *FileConfig) *Ref {
	return &Ref{config: initial}
}

func (r *Ref) Get() *FileConfig {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.config
}

func (r *Ref) Set(cfg *FileConfig) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.config = cfg
}
