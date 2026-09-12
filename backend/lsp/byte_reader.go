package lsp

import (
	"bufio"
	"fmt"
	"io"
)

// byteReader provides line/byte reads over a bufio.Reader; os.File-like
// sources suffice for both real pipes and normal files.
type byteReader struct {
	r io.Reader
}

func (b *byteReader) readLine() ([]byte, error) {
	if br, ok := b.r.(*bufio.Reader); ok {
		return br.ReadBytes('\n')
	}
	br := bufio.NewReader(b.r)
	b.r = br
	return br.ReadBytes('\n')
}

func (b *byteReader) readN(n int) ([]byte, error) {
	buf := make([]byte, n)
	if _, err := io.ReadFull(b.r, buf); err != nil {
		return nil, fmt.Errorf("lsp: short read (%d bytes): %w", n, err)
	}
	return buf, nil
}
