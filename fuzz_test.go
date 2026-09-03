package tomledit

import (
	"bytes"
	"testing"
)

// fuzzSeeds is the one seed corpus every fuzz target in this package adds.
// Keeping a single list stops the per-target lists from drifting apart, so a
// seed added for one target exercises all of them. Crashers found by the
// fuzzer are committed under testdata/fuzz/<Target>/ instead, per Go's own
// convention -- never appended here.
var fuzzSeeds = [][]byte{
	[]byte(`key = "value"` + "\n"),
	[]byte("[table]\nkey = 42\n"),
	[]byte(`[[array]]` + "\nname = \"item\"\n"),
	[]byte(`key = {a = 1, b = "two"}` + "\n"),
	[]byte("key = [1, 2, 3]\n"),
	[]byte("key = 2024-01-15T10:30:00Z\n"),
	[]byte(`key = """multi\nline"""` + "\n"),
	[]byte("# just a comment\n"),
	[]byte(""),
	[]byte("key = true\n"),
	[]byte("key = false\n"),
	[]byte("key = 3.14\n"),
	[]byte("key = 0xff\n"),
	[]byte("key = 0o77\n"),
	[]byte("key = 0b1010\n"),
	[]byte("key = 'literal string'\n"),
	[]byte("key = '''\nmulti-line\nliteral\n'''\n"),
	[]byte("key = 1979-05-27T07:32:00\n"),
	[]byte("key = 1979-05-27\n"),
	[]byte("key = 07:32:00\n"),
	[]byte(`"quoted key" = "value"` + "\n"),
	[]byte("a.b.c = \"dotted\"\n"),
	[]byte("[a.b.c]\nkey = \"deep\"\n"),
	[]byte("\r\nkey = \"crlf\"\r\n"),
	[]byte("  key  =  \"spaces\"  \n"),
	[]byte("key = +inf\n"),
	[]byte("key = -inf\n"),
	[]byte("key = nan\n"),
	[]byte("key = +nan\n"),
	[]byte("key = -0\n"),
	[]byte("key = +0\n"),
	[]byte("[server]\nhost = \"localhost\"\nport = 8080\n\n[[products]]\nname = \"Widget\"\nprice = 9.99\n"),
	[]byte(`"" = "empty key"` + "\n"),
}

// addFuzzSeeds seeds f with the shared corpus.
func addFuzzSeeds(f *testing.F) {
	f.Helper()
	for _, seed := range fuzzSeeds {
		f.Add(seed)
	}
}

// Fails if Parse panics on any seed or generated input.
func FuzzParse(f *testing.F) {
	addFuzzSeeds(f)

	f.Fuzz(func(t *testing.T, data []byte) {
		// Parse must not panic on any input
		Parse(data)
	})
}

// Fails if a parsed document re-renders into TOML that does not parse, or if a
// second render differs from the first.
func FuzzRoundTrip(f *testing.F) {
	addFuzzSeeds(f)

	f.Fuzz(func(t *testing.T, data []byte) {
		doc, err := Parse(data)
		if err != nil {
			return // Invalid input, skip
		}

		// Bytes() must not panic
		out := doc.Bytes()

		// Output must be valid TOML that parses
		doc2, err := Parse(out)
		if err != nil {
			t.Fatalf("round-trip produced invalid TOML: %v\ninput:  %q\noutput: %q", err, data, out)
		}

		// Second round-trip must be stable (Bytes of Bytes must equal Bytes)
		out2 := doc2.Bytes()
		if !bytes.Equal(out, out2) {
			t.Fatalf("round-trip not stable:\nfirst:  %q\nsecond: %q", out, out2)
		}
	})
}
