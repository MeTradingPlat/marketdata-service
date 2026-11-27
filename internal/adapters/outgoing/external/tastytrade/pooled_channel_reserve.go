package tastytrade

import "fmt"

func validateReserveKeys(keys []string) error {
	if len(keys) > channelCapacity {
		return fmt.Errorf("cannot reserve %d keys on channel (capacity %d)", len(keys), channelCapacity)
	}
	return nil
}

func (p *pooledChannel) countNewLocked(keys []string) int {
	if len(keys) == 1 {
		if _, exists := p.symbols[keys[0]]; exists {
			return 0
		}
		return 1
	}
	seen := make(map[string]struct{}, len(keys))
	needed := 0
	for _, k := range keys {
		if _, exists := p.symbols[k]; exists {
			continue
		}
		if _, dup := seen[k]; dup {
			continue
		}
		seen[k] = struct{}{}
		needed++
	}
	return needed
}

func (p *pooledChannel) canFit(keys ...string) bool {
	if p.isRetired() {
		return false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(keys) == 0 {
		return len(p.symbols) < channelCapacity
	}
	return len(p.symbols)+p.countNewLocked(keys) <= channelCapacity
}

func (p *pooledChannel) reserve(keys ...string) bool {
	if p.isRetired() {
		return false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(keys) == 0 {
		return len(p.symbols) < channelCapacity
	}
	if len(p.symbols)+p.countNewLocked(keys) > channelCapacity {
		return false
	}
	for _, k := range keys {
		p.symbols[k] = struct{}{}
	}
	return true
}
