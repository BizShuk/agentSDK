package core

import (
	"sync"
	"time"
)

// TodoStatus is the lifecycle stage of a remediation todo.
type TodoStatus string

const (
	TODO_STATUS_OPEN       TodoStatus = "open"
	TODO_STATUS_IN_PROGRESS TodoStatus = "in_progress"
	TODO_STATUS_DONE       TodoStatus = "done"
)

// Todo is one remediation task. Persisted in-memory only — M4 may
// serialize to disk for cross-run tracking.
type Todo struct {
	ID        string     `json:"id"`
	Title     string     `json:"title"`
	Status    TodoStatus `json:"status"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

// TodoStore is a thread-safe in-memory todo list. The runtime loop
// may add / list / complete concurrently with multiple tool calls; the
// mutex serializes the mutations.
type TodoStore struct {
	mu    sync.Mutex
	next  int
	items []Todo
}

// NewTodoStore returns an empty store.
func NewTodoStore() *TodoStore { return &TodoStore{} }

// Add appends a new open todo with the given title.
func (s *TodoStore) Add(title string) Todo {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.next++
	t := Todo{
		ID:        idPrefix(s.next),
		Title:     title,
		Status:    TODO_STATUS_OPEN,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	s.items = append(s.items, t)
	return t
}

// List returns a snapshot of all todos (copy — caller may mutate).
func (s *TodoStore) List() []Todo {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Todo, len(s.items))
	copy(out, s.items)
	return out
}

// Complete marks the todo with id done. Returns the updated todo or
// (zero, false) when the id is unknown.
func (s *TodoStore) Complete(id string) (Todo, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.items {
		if s.items[i].ID == id {
			s.items[i].Status = TODO_STATUS_DONE
			s.items[i].UpdatedAt = time.Now().UTC()
			return s.items[i], true
		}
	}
	return Todo{}, false
}

// Open returns only open todos — handy for the agent's "what's left?" prompt.
func (s *TodoStore) Open() []Todo {
	all := s.List()
	out := all[:0]
	for _, t := range all {
		if t.Status != TODO_STATUS_DONE {
			out = append(out, t)
		}
	}
	return out
}

func idPrefix(n int) string {
	return "todo-" + itoa(n)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}