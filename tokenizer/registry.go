package tokenizer

import "sync"

/*
Registry stores process-local tokenizer artifacts for graph operations.
*/
type Registry struct {
	mu      sync.RWMutex
	entries map[string]*Artifact
}

var globalRegistry = &Registry{entries: make(map[string]*Artifact)}

/*
GlobalRegistry returns the process-local tokenizer registry.
*/
func GlobalRegistry() *Registry {
	return globalRegistry
}

/*
Get retrieves an artifact by source key.
*/
func (registry *Registry) Get(source Source) (*Artifact, bool) {
	registry.mu.RLock()
	defer registry.mu.RUnlock()

	artifact, ok := registry.entries[source.Key()]

	return artifact, ok
}

/*
Store adds an artifact to the registry.
*/
func (registry *Registry) Store(artifact *Artifact) {
	registry.mu.Lock()
	defer registry.mu.Unlock()

	registry.entries[artifact.Source.Key()] = artifact
}

/*
StoreForTest seeds the registry with a synthetic tokenizer.
*/
func (registry *Registry) StoreForTest(source Source, tokenizer Tokenizer) {
	artifact := &Artifact{
		Source:    source.WithDefaults(),
		Backend:   "test",
		Tokenizer: tokenizer,
	}

	registry.Store(artifact)
}
