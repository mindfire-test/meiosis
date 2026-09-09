package graph

import (
	"errors"

	"github.com/mindfire-test/meiosis/pkg/storage"
)

var ErrInactiveAttempt = errors.New("attempt is not active")

// Store persists validated Meiosis objects through a backend-neutral store.
type Store struct {
	backend storage.Store
}
