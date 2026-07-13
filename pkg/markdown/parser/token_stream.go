package parser

import "github.com/cjccjj/mdflow/pkg/markdown/tokenizer"

// tokenBuffer owns the parser bounded streaming cursor.
type tokenBuffer struct {
	buf []tokenizer.Token
	eof bool
}

func (p *Parser) appendTokens(tokens []tokenizer.Token) { p.buf = append(p.buf, tokens...) }
func (p *Parser) bufferedLen() int                      { return len(p.buf) }
func (p *Parser) hasBufferedTokens() bool               { return len(p.buf) > 0 }

func (p *Parser) prependTokens(tokens ...tokenizer.Token) {
	if len(tokens) != 0 {
		p.buf = append(append([]tokenizer.Token(nil), tokens...), p.buf...)
	}
}

func (p *Parser) consume(n int) {
	if n > len(p.buf) {
		n = len(p.buf)
	}
	p.buf = p.buf[n:]
}
