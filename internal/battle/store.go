package battle

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sync"
)

// Store serializes read-modify-write transactions. Run a single server per data directory.
// A random 192-bit battle ID is a bearer capability; there is deliberately no list API.
type Store struct {
	mu  sync.Mutex
	dir string
}

var battleID = regexp.MustCompile(`^[a-f0-9]{48}$`)

func NewStore(dir string) *Store {
	if dir == "" {
		dir = "data"
	}
	return &Store{dir: dir}
}
func (s *Store) save(b *Battle) error {
	if err := os.MkdirAll(s.dir, 0700); err != nil {
		return err
	}
	raw, err := json.Marshal(b)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(s.dir, ".battle-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if _, err = f.Write(raw); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), filepath.Join(s.dir, b.ID+".json"))
}
func (s *Store) Create() (View, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b := NewBattle()
	if err := s.save(b); err != nil {
		return View{}, fmt.Errorf("save battle: %w", err)
	}
	return b.View(), nil
}
func (s *Store) Access(id string, change func(*Battle) error) (View, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !battleID.MatchString(id) {
		return View{}, fmt.Errorf("invalid battle_id")
	}
	raw, err := os.ReadFile(filepath.Join(s.dir, id+".json"))
	if err != nil {
		return View{}, fmt.Errorf("battle unavailable; check battle_id or create a new battle")
	}
	var b Battle
	if err = json.Unmarshal(raw, &b); err != nil {
		return View{}, fmt.Errorf("cannot read saved battle")
	}
	if change != nil {
		if err = change(&b); err != nil {
			return View{}, err
		}
		if err = s.save(&b); err != nil {
			return View{}, fmt.Errorf("save failed; refresh before retrying: %w", err)
		}
	}
	return b.View(), nil
}
