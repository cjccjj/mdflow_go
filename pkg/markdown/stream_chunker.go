package markdown

import (
	"bytes"
	"unicode/utf8"
)

// streamChunker turns arbitrary Write chunks into newline-delimited parser input
// while retaining only an incomplete trailing UTF-8 sequence.
type streamChunker struct {
	utf8Buf []byte
}

func (c *streamChunker) Write(data []byte, handle func([]byte) error) error {
	if len(c.utf8Buf) > 0 {
		data = append(c.utf8Buf, data...)
		c.utf8Buf = nil
	}

	if len(data) > 0 {
		start := len(data) - 1
		for i := 0; i < 4 && start >= 0; i, start = i+1, start-1 {
			if utf8.RuneStart(data[start]) {
				break
			}
		}
		if start >= 0 && start < len(data) && utf8.RuneStart(data[start]) && !utf8.FullRune(data[start:]) {
			c.utf8Buf = append([]byte(nil), data[start:]...)
			data = data[:start]
		}
	}

	for len(data) > 0 {
		idx := bytes.IndexByte(data, '\n')
		if idx < 0 {
			if err := handle(data); err != nil {
				return err
			}
			break
		}
		if err := handle(data[:idx+1]); err != nil {
			return err
		}
		data = data[idx+1:]
	}
	return nil
}

func (c *streamChunker) Drain(handle func([]byte) error) error {
	if len(c.utf8Buf) == 0 {
		return nil
	}
	data := c.utf8Buf
	c.utf8Buf = nil
	return handle(data)
}

func (c *streamChunker) Reset() { c.utf8Buf = nil }
