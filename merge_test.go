package tomledit

import (
	"strings"
	"testing"
)

// Helper to parse and fail on error.
func parseMergeDoc(t *testing.T, input string) *Document {
	t.Helper()
	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}
	return doc
}

// Test 7: Merge two documents -- basic document merge.
func TestMerge_TwoDocuments(t *testing.T) {
	target := parseMergeDoc(t, `[server]
host = "myhost"
port = 9090
`)
	source := parseMergeDoc(t, `[server]
host = "default-host"
workers = 4

[logging]
level = "debug"
`)
	err := target.Merge(source)
	if err != nil {
		t.Fatalf("Merge returned error: %v", err)
	}

	// Existing keys unchanged.
	host, _ := target.GetString("server.host")
	if host != "myhost" {
		t.Errorf("expected host %q, got %q", "myhost", host)
	}
	port, err := target.GetInt("server.port")
	if err != nil || port != 9090 {
		t.Errorf("expected port %d, got %d", 9090, port)
	}

	// New keys added.
	workers, err := target.GetInt("server.workers")
	if err != nil || workers != 4 {
		t.Errorf("expected workers %d, got %d (ok=%v)", 4, workers, err)
	}

	level, err := target.GetString("logging.level")
	if err != nil || level != "debug" {
		t.Errorf("expected logging.level %q, got %q (ok=%v)", "debug", level, err)
	}

	// Round-trip.
	out := target.Bytes()
	if _, err := Parse(out); err != nil {
		t.Fatalf("round-trip parse failed: %v\noutput:\n%s", err, string(out))
	}
}

// Test 8: Merge with comments -- source comments appended to target comments for existing keys.
func TestMerge_WithComments(t *testing.T) {
	target := parseMergeDoc(t, `# Target comment
[server]
# Target host comment
host = "myhost"
`)
	source := parseMergeDoc(t, `# Source comment
[server]
# Source host comment
host = "default-host"
`)
	err := target.Merge(source)
	if err != nil {
		t.Fatalf("Merge returned error: %v", err)
	}

	// Value unchanged.
	host, _ := target.GetString("server.host")
	if host != "myhost" {
		t.Errorf("expected host %q, got %q", "myhost", host)
	}

	// Check that source comments were appended.
	out := string(target.Bytes())
	if !strings.Contains(out, "Target host comment") {
		t.Errorf("target comment was lost, got:\n%s", out)
	}
	if !strings.Contains(out, "Source host comment") {
		t.Errorf("source comment was not appended, got:\n%s", out)
	}
}

// Test 9: Merge new keys with comments -- new keys bring their comments.
func TestMerge_NewKeysWithComments(t *testing.T) {
	target := parseMergeDoc(t, `[server]
host = "myhost"
`)
	source := parseMergeDoc(t, `[server]
# Worker count for parallelism
workers = 4
`)
	err := target.Merge(source)
	if err != nil {
		t.Fatalf("Merge returned error: %v", err)
	}

	workers, err := target.GetInt("server.workers")
	if err != nil || workers != 4 {
		t.Errorf("expected workers %d, got %d (ok=%v)", 4, workers, err)
	}

	out := string(target.Bytes())
	if !strings.Contains(out, "Worker count for parallelism") {
		t.Errorf("source comment was not copied with new key, got:\n%s", out)
	}
}

// Test 10: Merge complex -- realistic config with multiple tables.
func TestMerge_Complex(t *testing.T) {
	target := parseMergeDoc(t, `title = "My App"

[server]
host = "prod.example.com"
port = 443

[database]
url = "postgres://prod:5432/mydb"
`)
	source := parseMergeDoc(t, `title = "Default App"
version = "1.0.0"

[server]
host = "localhost"
port = 8080
workers = 4
debug = false

[database]
url = "postgres://localhost/devdb"
pool_size = 5
timeout = 30

[logging]
level = "info"
format = "json"
`)
	err := target.Merge(source)
	if err != nil {
		t.Fatalf("Merge returned error: %v", err)
	}

	// Existing values unchanged.
	title, _ := target.GetString("title")
	if title != "My App" {
		t.Errorf("title overwritten: %q", title)
	}
	host, _ := target.GetString("server.host")
	if host != "prod.example.com" {
		t.Errorf("server.host overwritten: %q", host)
	}
	port, _ := target.GetInt("server.port")
	if port != 443 {
		t.Errorf("server.port overwritten: %d", port)
	}
	dbURL, _ := target.GetString("database.url")
	if dbURL != "postgres://prod:5432/mydb" {
		t.Errorf("database.url overwritten: %q", dbURL)
	}

	// New values added.
	version, err := target.GetString("version")
	if err != nil || version != "1.0.0" {
		t.Errorf("version not added: %q (ok=%v)", version, err)
	}
	workers, err := target.GetInt("server.workers")
	if err != nil || workers != 4 {
		t.Errorf("server.workers not added: %d (ok=%v)", workers, err)
	}
	debug, err := target.GetBool("server.debug")
	if err != nil || debug != false {
		t.Errorf("server.debug not added: %v (ok=%v)", debug, err)
	}
	poolSize, err := target.GetInt("database.pool_size")
	if err != nil || poolSize != 5 {
		t.Errorf("database.pool_size not added: %d (ok=%v)", poolSize, err)
	}
	timeout, err := target.GetInt("database.timeout")
	if err != nil || timeout != 30 {
		t.Errorf("database.timeout not added: %d (ok=%v)", timeout, err)
	}
	level, err := target.GetString("logging.level")
	if err != nil || level != "info" {
		t.Errorf("logging.level not added: %q (ok=%v)", level, err)
	}
	format, err := target.GetString("logging.format")
	if err != nil || format != "json" {
		t.Errorf("logging.format not added: %q (ok=%v)", format, err)
	}

	// Round-trip.
	out := target.Bytes()
	if _, err := Parse(out); err != nil {
		t.Fatalf("round-trip parse failed: %v\noutput:\n%s", err, string(out))
	}
}
