package audit

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

// member is a validated, normalized non-whiteout tar entry.
type member struct {
	path   string
	kind   Kind
	sha256 string // files only
	target string // symlinks only
}

// deletion removes the path (and its descendants) from lower layers.
type deletion struct {
	path string
}

// parsedLayer is the validated content of one tar layer.
type parsedLayer struct {
	members   []member
	deletions []deletion
	opaque    []string // directories whose lower-layer descendants are cleared
}

// ParseLayer reads and fully validates one uncompressed tar layer.
// Nothing is applied to any state here; parsing is pure.
func ParseLayer(r io.Reader) (*parsedLayer, error) {
	tr := tar.NewReader(r)
	pl := &parsedLayer{}
	seen := map[string]bool{}
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("reading tar: %w", err)
		}
		p, err := NormalizePath(hdr.Name)
		if err != nil {
			return nil, err
		}
		if seen[p] {
			return nil, fmt.Errorf("duplicate member %q in layer", p)
		}
		seen[p] = true

		base := path.Base(p)
		if base == opaqueMarker {
			if hdr.Typeflag != tar.TypeReg || hdr.Size != 0 {
				return nil, fmt.Errorf("opaque marker %q must be an empty regular file", p)
			}
			dir := ParentOf(p)
			pl.opaque = append(pl.opaque, dir)
			continue
		}
		if strings.HasPrefix(base, whiteoutPrefix) {
			if hdr.Typeflag != tar.TypeReg || hdr.Size != 0 {
				return nil, fmt.Errorf("whiteout %q must be an empty regular file", p)
			}
			target := path.Join(ParentOf(p), base[len(whiteoutPrefix):])
			pl.deletions = append(pl.deletions, deletion{path: target})
			continue
		}

		switch hdr.Typeflag {
		case tar.TypeReg:
			h := sha256.New()
			if _, err := io.Copy(h, tr); err != nil {
				return nil, fmt.Errorf("reading %q: %w", p, err)
			}
			pl.members = append(pl.members, member{path: p, kind: KindFile, sha256: hex.EncodeToString(h.Sum(nil))})
		case tar.TypeDir:
			pl.members = append(pl.members, member{path: p, kind: KindDir})
		case tar.TypeSymlink:
			pl.members = append(pl.members, member{path: p, kind: KindSymlink, target: hdr.Linkname})
		default:
			return nil, fmt.Errorf("member %q has unsupported type %d", p, hdr.Typeflag)
		}
	}
	return pl, nil
}

// ParseLayerFile opens and parses an uncompressed tar layer from disk.
func ParseLayerFile(name string) (*parsedLayer, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return ParseLayer(f)
}
