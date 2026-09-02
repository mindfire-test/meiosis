package v1

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
)

const WorldHashSize = 32

// WorldHash is the canonical 32-byte content hash for a tree/world state.
type WorldHash [WorldHashSize]byte

func ParseWorldHash(s string) (WorldHash, error) {
	var h WorldHash
	b, err := hex.DecodeString(s)
	if err != nil || len(b) != WorldHashSize {
		return h, ErrInvalidWorldHash
	}
	copy(h[:], b)
	return h, nil
}

func MustParseWorldHash(s string) WorldHash {
	h, err := ParseWorldHash(s)
	if err != nil {
		panic(err)
	}
	return h
}

func (h WorldHash) String() string {
	return hex.EncodeToString(h[:])
}

func (h WorldHash) Validate() error {
	return nil
}

func (h WorldHash) MarshalJSON() ([]byte, error) {
	return json.Marshal(h.String())
}

func (h *WorldHash) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return fmt.Errorf("%w: expected string", ErrInvalidWorldHash)
	}
	parsed, err := ParseWorldHash(s)
	if err != nil {
		return err
	}
	*h = parsed
	return nil
}
