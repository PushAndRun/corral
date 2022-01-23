package corral

import (
	"sync"
)

type Set struct {
	m map[interface{}]bool
	sync.RWMutex
}

func NewConcurrentSet() *Set {
	return &Set{
		m: make(map[interface{}]bool),
	}
}

// Add add
func (s *Set) Add(item interface{}) {
	s.Lock()
	defer s.Unlock()
	s.m[item] = true
}

// Remove deletes the specified item from the map
func (s *Set) Remove(item interface{}) {
	s.Lock()
	defer s.Unlock()
	delete(s.m, item)
}

// Has looks for the existence of an item
func (s *Set) Has(item interface{}) bool {
	s.RLock()
	defer s.RUnlock()
	_, ok := s.m[item]
	return ok
}

// Len returns the number of items in a set.
func (s *Set) Len() int {
	s.RLock()
	defer s.RUnlock()
	length := len(s.m)
	return length
}

// Clear removes all items from the set
func (s *Set) Clear() {
	s.Lock()
	defer s.Unlock()
	s.m = make(map[interface{}]bool)
}

// IsEmpty checks for emptiness
func (s *Set) IsEmpty() bool {
	if s.Len() == 0 {
		return true
	}
	return false
}

func (s *Set) AddAll(all []interface{}) {
	s.Lock()
	defer s.Unlock()
	for _, e := range all {
		s.m[e] = true
	}
}
