package tomledit

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ParseFile reads a document from disk and the document remembers where it came
// from, so that every diagnostic it produces afterwards names the file.

// writeTOML writes content to a file in a temporary directory and returns its
// path.
func writeTOML(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
	return path
}

// Fails if ParseFile stops producing the same document Parse produces for the
// same bytes.
func TestParseFile(t *testing.T) {
	const content = "# a config\ntitle = \"hello\"\n\n[server]\nhost = \"localhost\"\n"
	path := writeTOML(t, "config.toml", content)

	doc, err := ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile failed: %v", err)
	}
	if got := string(doc.Bytes()); got != content {
		t.Errorf("round-trip = %q, want %q", got, content)
	}
	if host, ok := doc.GetString("server.host"); !ok || host != "localhost" {
		t.Errorf("server.host = %q, %v; want \"localhost\", true", host, ok)
	}
}

// Fails if an unreadable file stops being reported as the underlying read
// error -- nothing was parsed, so there is nothing to diagnose.
func TestParseFileMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "absent.toml")
	doc, err := ParseFile(path)
	if err == nil {
		t.Fatalf("ParseFile on a missing file returned %v, want an error", doc)
	}
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("errors.Is(err, fs.ErrNotExist) = false, want true (err: %v)", err)
	}
	var diag *Error
	if errors.As(err, &diag) {
		t.Errorf("a read failure must not be reported as a diagnostic, got %v", diag)
	}
}

// Fails if a parse failure from ParseFile stops naming the file it came from.
func TestParseFileSyntaxErrorNamesTheFile(t *testing.T) {
	path := writeTOML(t, "broken.toml", "a = 1\na = 2\n")

	_, err := ParseFile(path)
	if err == nil {
		t.Fatalf("expected a parse error")
	}
	var diag *Error
	if !errors.As(err, &diag) {
		t.Fatalf("expected *Error, got %T: %v", err, err)
	}
	if diag.File != path {
		t.Errorf("File = %q, want %q", diag.File, path)
	}
	if !strings.HasPrefix(err.Error(), path+":") {
		t.Errorf("rendered error %q does not start with the file name", err.Error())
	}
}

// Fails if a later diagnostic -- from access, editing or commenting -- stops
// naming the file the document was read from.
func TestParseFileDiagnosticsNameTheFile(t *testing.T) {
	const content = "title = \"hello\"\npoint = { x = 1 }\n\n[server]\nhost = \"localhost\"\n"
	path := writeTOML(t, "config.toml", content)

	tests := []struct {
		name string
		run  func(d *Document) error
	}{
		{"resolve", func(d *Document) error { _, err := d.Resolve("server.port"); return err }},
		{"set", func(d *Document) error { return d.Set("nope.deep", 1) }},
		{"delete with a bad path", func(d *Document) error { return d.Delete("a[") }},
		{"rename", func(d *Document) error { return d.RenameKey("server.port", "p") }},
		{"new table", func(d *Document) error { return d.NewTable("server") }},
		{"comment", func(d *Document) error { return d.SetComment("point.x", "no") }},
		{"cursor", func(d *Document) error { return d.Key("server").Key("port").Err() }},
		{"ensure defaults", func(d *Document) error {
			_, err := d.EnsureDefaults([]Default{{Path: "fresh", Value: make(chan int)}})
			return err
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc, err := ParseFile(path)
			if err != nil {
				t.Fatalf("ParseFile failed: %v", err)
			}
			err = tt.run(doc)
			if err == nil {
				t.Fatalf("expected an error")
			}
			var diag *Error
			if !errors.As(err, &diag) {
				t.Fatalf("expected *Error, got %T: %v", err, err)
			}
			if diag.File != path {
				t.Errorf("File = %q, want %q (err: %v)", diag.File, path, err)
			}
			if !strings.HasPrefix(err.Error(), path+":") {
				t.Errorf("rendered error %q does not start with the file name", err.Error())
			}
		})
	}
}

// Fails if a document parsed from bytes starts claiming a file.
func TestParseCarriesNoFile(t *testing.T) {
	doc, err := Parse([]byte("title = \"hello\"\n"))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	err = doc.Set("nope.deep", 1)
	if err == nil {
		t.Fatalf("expected an error")
	}
	var diag *Error
	if !errors.As(err, &diag) {
		t.Fatalf("expected *Error, got %T: %v", err, err)
	}
	if diag.File != "" {
		t.Errorf("File = %q, want empty for a document parsed from bytes", diag.File)
	}
}

// Fails if a DecodeNode diagnostic stops naming the file its node came from.
// A node reaches its document through the parent links the structure funnel
// maintains, which is how a decode scoped to one construct still knows where
// the construct was read from.
func TestParseFileDecodeNodeDiagnosticsNameTheFile(t *testing.T) {
	const content = "[server]\nport = \"eighty\"\n"
	path := writeTOML(t, "config.toml", content)

	doc, err := ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile failed: %v", err)
	}
	node, ok := doc.Lookup("server")
	if !ok {
		t.Fatal("server not found")
	}

	var target struct {
		Port int `toml:"port"`
	}
	err = DecodeNode(node, &target)
	if err == nil {
		t.Fatal("expected an error decoding a string into an int field")
	}
	var diag *Error
	if !errors.As(err, &diag) {
		t.Fatalf("expected *Error, got %T: %v", err, err)
	}
	if diag.File != path {
		t.Errorf("File = %q, want %q (err: %v)", diag.File, path, err)
	}
	if !strings.HasPrefix(err.Error(), path+":") {
		t.Errorf("Error() = %q, want it to start with %q", err.Error(), path+":")
	}
}
