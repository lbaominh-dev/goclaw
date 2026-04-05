package localworker

import "sync"

type WaiterRegistry struct {
	mu   sync.RWMutex
	subs map[string]map[chan WorkerReplyEnvelope]struct{}
}

func NewWaiterRegistry() *WaiterRegistry {
	return &WaiterRegistry{subs: make(map[string]map[chan WorkerReplyEnvelope]struct{})}
}

func (r *WaiterRegistry) Subscribe(jobID string) chan WorkerReplyEnvelope {
	ch := make(chan WorkerReplyEnvelope, 16)
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.subs[jobID] == nil {
		r.subs[jobID] = make(map[chan WorkerReplyEnvelope]struct{})
	}
	r.subs[jobID][ch] = struct{}{}
	return ch
}

func (r *WaiterRegistry) Unsubscribe(jobID string, ch chan WorkerReplyEnvelope) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if subs := r.subs[jobID]; subs != nil {
		delete(subs, ch)
		if len(subs) == 0 {
			delete(r.subs, jobID)
		}
	}
	close(ch)
}

func (r *WaiterRegistry) Publish(jobID string, msg WorkerReplyEnvelope) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for ch := range r.subs[jobID] {
		select {
		case ch <- msg:
		default:
		}
	}
}
