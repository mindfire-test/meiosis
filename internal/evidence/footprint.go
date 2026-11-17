package evidence

import (
	"path"
	"sort"
	"strings"

	specv1 "github.com/mindfire-test/meiosis/pkg/spec/v1"
)

// A footprint taken from a report must never be smaller than the truth: a
// missing file would let evidence stay carried after that file changes. So the
// rule is all or nothing. Either every file the report names maps onto a clean
// repository path, or the report contributes no footprint at all and the
// evidence depends on the whole world.

// resolveFootprint turns the file names a report lists into repository paths.
// root is the repository's absolute path, used to relativise absolute names;
// without it an absolute name cannot be resolved. It returns nil when any name
// cannot be resolved, when there are none, or when there are more than
// specv1.MaxFootprintEntries.
func resolveFootprint(names []string, root string) []string {
	if len(names) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(names))
	out := make([]string, 0, len(names))
	for _, name := range names {
		p, ok := repoPath(name, root)
		if !ok {
			return nil
		}
		if !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	if len(out) > specv1.MaxFootprintEntries {
		return nil
	}
	sort.Strings(out)
	return out
}

// repoPath maps one file name from a report onto a repository path.
func repoPath(name, root string) (string, bool) {
	p := strings.ReplaceAll(strings.TrimSpace(name), `\`, "/")
	if len(p) >= 2 && p[1] == ':' {
		return "", false // a Windows drive path; it cannot be relativised here
	}
	if strings.HasPrefix(p, "/") {
		r := strings.TrimSuffix(path.Clean(strings.ReplaceAll(root, `\`, "/")), "/")
		if root == "" || !strings.HasPrefix(p, r+"/") {
			return "", false // outside the repository, or no root to resolve against
		}
		p = strings.TrimPrefix(p, r+"/")
	}
	p = path.Clean(p)
	// A real file name with glob characters ("app/[id]/page.tsx") would be read
	// back as a pattern that does not match the file itself.
	if strings.ContainsAny(p, "*?[{") || !specv1.ValidRepoPath(p) {
		return "", false
	}
	return p, true
}
