package model

import (
	"sync"
)

type Store struct {
	sync.RWMutex
	items map[string]interface{}
	keys []string
}

func NewStore() *Store {
	return &Store{items: make(map[string]interface{})}
}

func (s *Store) Save(key string, item interface{}) {
	s.Lock()
	defer s.Unlock()
	if _, found := s.items[key]; !found {
		s.keys = append(s.keys, key)
	}
	s.items[key] = item
}

func (s *Store) Get(key string) (interface{}, bool) {
	s.Lock()
	defer s.Unlock()
	item, found := s.items[key]
	return item, found
}

func (s *Store) Keys() []string {
	s.RLock()
	defer s.RUnlock()
	return s.keys
}

func (s *Store) Remove(key string) {
	s.Lock()
	defer s.Unlock()
	delete(s.items, key)
	for i := range s.keys {
		if s.keys[i] == key{
			s.keys = append(s.keys[0:i], s.keys[i+1:]...)
			break
		}
	}
}
