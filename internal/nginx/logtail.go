package nginx

import (
	"bytes"
	"io"
	"os"
)

const (
	defaultTailBytes = 256 * 1024 // never read more than this from the tail
	maxTailLines     = 2000
)

// tailFile returns up to `lines` of the last lines of path, reading at most
// `maxBytes` from the end of the file. A missing file yields an empty slice
// (a site with no traffic has no log yet), not an error.
func tailFile(path string, lines, maxBytes int) ([]string, error) {
	if lines < 1 {
		lines = 1
	}
	if lines > maxTailLines {
		lines = maxTailLines
	}
	if maxBytes <= 0 || maxBytes > defaultTailBytes {
		maxBytes = defaultTailBytes
	}

	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}
	defer f.Close()

	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	start := int64(0)
	if st.Size() > int64(maxBytes) {
		start = st.Size() - int64(maxBytes)
	}
	if _, err := f.Seek(start, io.SeekStart); err != nil {
		return nil, err
	}
	buf, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}
	if start > 0 { // drop the partial first line
		if i := bytes.IndexByte(buf, '\n'); i >= 0 {
			buf = buf[i+1:]
		}
	}
	buf = bytes.TrimRight(buf, "\n")
	if len(buf) == 0 {
		return []string{}, nil
	}
	all := bytes.Split(buf, []byte{'\n'})
	if len(all) > lines {
		all = all[len(all)-lines:]
	}
	out := make([]string, len(all))
	for i, l := range all {
		out[i] = string(l)
	}
	return out, nil
}
