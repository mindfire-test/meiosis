package worlds

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"path"
	"sort"
	"strings"

	specv1 "github.com/mindfire-test/meiosis/pkg/spec/v1"
	"github.com/zeebo/blake3"
)

// World is an immutable Meiosis snapshot. Tree implementations own their
// storage strategy; structural sharing remains outside this package.
type World struct {
	tree Tree
	hash specv1.WorldHash
}

// New creates a world from a tree snapshot and computes its deterministic
// BLAKE3-256 WorldHash.
func New(ctx context.Context, tree Tree) (World, error) {
	if tree == nil {
		return World{}, ErrNilTree
	}
	files, err := tree.Files(ctx)
	if err != nil {
		return World{}, fmt.Errorf("read world tree: %w", err)
	}
	hash, err := hashFiles(files)
	if err != nil {
		return World{}, err
	}
	return World{tree: tree, hash: hash}, nil
}

// Snapshot obtains a tree from source and creates a world from it.
func Snapshot(ctx context.Context, source Snapshotter) (World, error) {
	if source == nil {
		return World{}, ErrNilTree
	}
	tree, err := source.Snapshot(ctx)
	if err != nil {
		return World{}, fmt.Errorf("snapshot world: %w", err)
	}
	return New(ctx, tree)
}

// Hash returns the content address of the world.
func (w World) Hash() specv1.WorldHash {
	return w.hash
}

// Tree returns the backend tree associated with the world.
func (w World) Tree() (Tree, error) {
	if w.tree == nil {
		return nil, ErrMissingWorld
	}
	return w.tree, nil
}

// Fork creates a new snapshot with the same content. A production Tree may
// implement its own fork operation and structural sharing behind this API.
func (w World) Fork(ctx context.Context) (World, error) {
	if w.tree == nil {
		return World{}, ErrMissingWorld
	}
	if forkable, ok := w.tree.(ForkableTree); ok {
		tree, err := forkable.Fork(ctx)
		if err != nil {
			return World{}, fmt.Errorf("fork world tree: %w", err)
		}
		return New(ctx, tree)
	}
	return New(ctx, w.tree)
}

// Diff returns deterministic file-level changes from w to other.
func (w World) Diff(ctx context.Context, other World) ([]Change, error) {
	if w.tree == nil || other.tree == nil {
		return nil, ErrMissingWorld
	}
	before, err := w.tree.Files(ctx)
	if err != nil {
		return nil, fmt.Errorf("read base world: %w", err)
	}
	after, err := other.tree.Files(ctx)
	if err != nil {
		return nil, fmt.Errorf("read target world: %w", err)
	}
	paths := make(map[string]struct{}, len(before)+len(after))
	for filePath := range before {
		paths[filePath] = struct{}{}
	}
	for filePath := range after {
		paths[filePath] = struct{}{}
	}
	ordered := make([]string, 0, len(paths))
	for filePath := range paths {
		ordered = append(ordered, filePath)
	}
	sort.Strings(ordered)

	changes := make([]Change, 0)
	for _, filePath := range ordered {
		oldValue, existedBefore := before[filePath]
		newValue, existsAfter := after[filePath]
		switch {
		case !existedBefore:
			changes = append(changes, Change{Path: filePath, Kind: Added, After: cloneBytes(newValue)})
		case !existsAfter:
			changes = append(changes, Change{Path: filePath, Kind: Deleted, Before: cloneBytes(oldValue)})
		case !bytes.Equal(oldValue, newValue):
			changes = append(changes, Change{Path: filePath, Kind: Modified, Before: cloneBytes(oldValue), After: cloneBytes(newValue)})
		}
	}
	return changes, nil
}

func hashFiles(files map[string][]byte) (specv1.WorldHash, error) {
	// This is a temporary deterministic file-tree encoding. The eventual
	// zygote/vfs adapter will own production Merkle-tree semantics.
	paths := make([]string, 0, len(files))
	for filePath := range files {
		if err := validatePath(filePath); err != nil {
			return specv1.WorldHash{}, err
		}
		paths = append(paths, filePath)
	}
	sort.Strings(paths)
	hasher := blake3.New()
	var length [8]byte
	for _, filePath := range paths {
		content := files[filePath]
		binary.BigEndian.PutUint64(length[:], uint64(len(filePath)))
		_, _ = hasher.Write(length[:])
		_, _ = hasher.Write([]byte(filePath))
		binary.BigEndian.PutUint64(length[:], uint64(len(content)))
		_, _ = hasher.Write(length[:])
		_, _ = hasher.Write(content)
	}
	digest := hasher.Sum(nil)
	var hash specv1.WorldHash
	copy(hash[:], digest)
	return hash, nil
}

func validatePath(filePath string) error {
	if filePath == "" || strings.HasPrefix(filePath, "/") || strings.Contains(filePath, "\\") {
		return fmt.Errorf("%w: %q", ErrInvalidPath, filePath)
	}
	clean := path.Clean(filePath)
	if clean != filePath || clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return fmt.Errorf("%w: %q", ErrInvalidPath, filePath)
	}
	return nil
}

func cloneBytes(value []byte) []byte {
	return append([]byte(nil), value...)
}
