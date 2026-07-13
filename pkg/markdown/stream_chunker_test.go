package markdown

import "testing"

func TestStreamChunkerPreservesUTF8AndLineBoundaries(t *testing.T) {
	var got []string
	handle := func(chunk []byte) error {
		got = append(got, string(chunk))
		return nil
	}

	var c streamChunker
	if err := c.Write([]byte{0xce}, handle); err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("incomplete rune was emitted: %q", got)
	}
	if err := c.Write([]byte{0xb1, 0x0a, 0x62, 0x65}, handle); err != nil {
		t.Fatal(err)
	}
	if err := c.Write([]byte("ta"), handle); err != nil {
		t.Fatal(err)
	}

	want := []string{"α\n", "be", "ta"}
	if len(got) != len(want) {
		t.Fatalf("chunks = %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("chunk %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestStreamChunkerDrainAndReset(t *testing.T) {
	var got string
	c := streamChunker{utf8Buf: []byte{0xce}}
	if err := c.Drain(func(chunk []byte) error { got = string(chunk); return nil }); err != nil {
		t.Fatal(err)
	}
	if got != string([]byte{0xce}) || len(c.utf8Buf) != 0 {
		t.Fatalf("drain got %q and retained %x", got, c.utf8Buf)
	}
	c.utf8Buf = []byte{0xb1}
	c.Reset()
	if len(c.utf8Buf) != 0 {
		t.Fatal("reset retained UTF-8 input")
	}
}
