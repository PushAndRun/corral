package corral

import (
	"fmt"
	"sort"
	"sync"
	"time"
)

type ActivationSet struct {
	//internal data structure
	m map[string]int64
	//flag to indicate this set is now immutable
	closed bool
	//mutex to wait on empty set
	notEmpty *sync.Cond
	//set mutex
	sync.RWMutex
}

func NewSet() *ActivationSet {
	m := &ActivationSet{
		m: make(map[string]int64),
	}
	m.notEmpty = sync.NewCond(&m.RWMutex)
	return m
}

// Add add
func (s *ActivationSet) Add(activationID string) error {
	s.Lock()
	defer s.Unlock()
	if s.closed {
		return fmt.Errorf(" cannot add to a closed set")
	}
	s.m[activationID] = time.Now().UnixMilli()
	s.notEmpty.Signal()
	return nil
}

// Remove deletes the specified item from the map
func (s *ActivationSet) Remove(activationID string) {
	s.Lock()
	defer s.Unlock()
	delete(s.m, activationID)
}

// Has looks for the existence of an item
func (s *ActivationSet) Has(activationID string) bool {
	s.RLock()
	defer s.RUnlock()
	_, ok := s.m[activationID]
	return ok
}

func (s *ActivationSet) Close() {
	s.Lock()
	defer s.Unlock()
	s.closed = true
}

func (s *ActivationSet) Drained(threshold int) bool {
	s.RLock()
	defer s.RUnlock()
	if s.closed {
		return len(s.m) < threshold
	}

	return false
}

// Len returns the number of items in a set.
func (s *ActivationSet) Len() int {
	s.RLock()
	defer s.RUnlock()
	return len(s.m)
}

// Clear removes all items from the set
func (s *ActivationSet) Clear() {
	s.Lock()
	defer s.Unlock()
	s.m = make(map[string]int64)

}

// IsEmpty checks for emptiness
func (s *ActivationSet) IsEmpty() bool {
	if s.Len() == 0 {
		return true
	}
	return false
}

func (s *ActivationSet) List() []string {
	s.RLock()
	defer s.RUnlock()
	list := make([]string, 0)
	for k, _ := range s.m {
		list = append(list, k)
	}
	return list
}

func (s *ActivationSet) AddAll(new []string) error {
	for i, v := range new {
		err := s.Add(v)
		if err != nil {
			return fmt.Errorf("error adding %s to set (added %d/%d): %+v", v, i, len(new), err)
		}
	}
	return nil
}

//Take returns the first num items from the set, blocking if necessary until all items are available
func (s *ActivationSet) Take(num int) []string {
	list := make([]string, 0)

	//we can't take num items, less than num available and set is closed
	if s.closed && s.Len() < num {
		s.Lock()
		for k, _ := range s.m {
			list = append(list, k)
			delete(s.m, k)
		}
		s.Unlock()
		return list
	}

	s.Lock()
	for len(list) < num {

		//if empty we wait until we can take at least one and lock
		if len(s.m) == 0 {
			s.notEmpty.Wait()
		}
		//Sort the current data by insertion time and return the first num items
		r := frozenSortedSet(s.m)
		for _, v := range r {
			list = append(list, v)
			delete(s.m, v)
			if len(list) == num {
				s.Unlock()
				return list
			}
		}

	}
	s.Unlock()
	return list
}

func frozenSortedSet(m map[string]int64) []string {
	type pair struct {
		key   string
		value int64
	}

	list := make([]pair, 0)

	for k, v := range m {
		list = append(list, pair{k, v})
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i].value < list[j].value
	})
	keys := make([]string, len(list))
	for i, p := range list {
		keys[i] = p.key
	}
	return keys
}
