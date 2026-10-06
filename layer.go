package auditor

import (
	"archive/tar"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
)

const (
	whiteoutPrefix = ".wh."
	opaqueMarker   = ".wh..wh..opq"
)

// member is one normalized non-whiteout tar member.
type member struct {
	path   string
	kind   Kind
	sha256 string
	target string
}

// parsedLayer is the validated content of one tar layer.
type parsedLayer struct {
	deletes  map[string]bool // paths removed by .wh. markers
	opaque   map[string]bool // dirs whose lower descendants are cleared
	members  []member        // sorted parent-first after validation
}

// parseLayer reads and fully validates one uncompressed tar layer.
func parseLayer(file string) (*parsedLayer, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	pl := &parsedLayer{
		deletes: map[string]bool{},
		opaque:  map[string]bool{},
	}
	seen := map[string]bool{}
	tr := tar.NewReader(f)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("%s: %w", file, err)
		}
		name, err := normalize(hdr.Name)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", file, err)
		}
		base := path.Base(name)
		if strings.HasPrefix(base, whiteoutPrefix) {
			if hdr.Typeflag != tar.TypeReg || hdr.Size != 0 {
				return nil, fmt.Errorf("%s: whiteout %q must be an empty regular file", file, name)
			}
			dir := parentOf(name)
			if base == opaqueMarker {
				pl.opaque[dir] = true
			} else {
				target := strings.TrimPrefix(base, whiteoutPrefix)
				if target == "" {
					return nil, fmt.Errorf("%s: invalid whiteout %q", file, name)
				}
				del := target
				if dir != "" {
					del = dir + "/" + target
				}
				pl.deletes[del] = true
			}
			continue
		}
		if seen[name] {
			return nil, fmt.Errorf("%s: duplicate member %q", file, name)
		}
		seen[name] = true
		m := member{path: name}
		switch hdr.Typeflag {
		case tar.TypeReg:
			m.kind = KindFile
			h := sha256.New()
			if _, err := io.Copy(h, tr); err != nil {
				return nil, fmt.Errorf("%s: %w", file, err)
			}
			m.sha256 = hex.EncodeToString(h.Sum(nil))
		case tar.TypeDir:
			m.kind = KindDir
		case tar.TypeSymlink:
			m.kind = KindSymlink
			m.target = hdr.Linkname
		default:
			return nil, fmt.Errorf("%s: unsupported entry type %d for %q", file, hdr.Typeflag, name)
		}
		pl.members = append(pl.members, m)
	}
	if err := pl.validate(); err != nil {
		return nil, fmt.Errorf("%s: %w", file, err)
	}
	return pl, nil
}

// validate checks intra-layer consistency and orders members parent-first.
func (pl *parsedLayer) validate() error {
	byPath := map[string]member{}
	for _, m := range pl.members {
		byPath[m.path] = m
	}
	// A whiteout must not delete a member added by the same layer.
	for del := range pl.deletes {
		if _, ok := byPath[del]; ok {
			return fmt.Errorf("whiteout deletes %q which is added by the same layer", del)
		}
	}
	// A non-directory member must not have descendants in the same layer.
	for _, m := range pl.members {
		if m.kind == KindDir {
			continue
		}
		for _, other := range pl.members {
			if isDescendantOf(other.path, m.path) {
				return fmt.Errorf("non-directory %q has descendant %q", m.path, other.path)
			}
		}
	}
	// Order parent-first so tar order does not affect the result.
	depth := func(p string) int { return strings.Count(p, "/") }
	ordered := append([]member(nil), pl.members...)
	for i := 1; i < len(ordered); i++ {
		for j := i; j > 0; j-- {
			a, b := ordered[j-1], ordered[j]
			if depth(a.path) > depth(b.path) || (depth(a.path) == depth(b.path) && a.path > b.path) {
				ordered[j-1], ordered[j] = b, a
			} else {
				break
			}
		}
	}
	pl.members = ordered
	return nil
}
