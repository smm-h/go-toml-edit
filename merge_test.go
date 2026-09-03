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

// Test 1: MergeDefaults basic -- existing keys unchanged, missing keys added.
func TestMergeDefaults_Basic(t *testing.T) {
	doc := parseMergeDoc(t, `[server]
host = "myhost"
port = 9090
`)
	defaults := map[string]any{
		"host":    "localhost",
		"port":    8080,
		"workers": 4,
	}
	err := doc.MergeDefaults("server", defaults)
	if err != nil {
		t.Fatalf("MergeDefaults returned error: %v", err)
	}

	// Existing keys should be unchanged.
	host, ok := doc.GetString("server.host")
	if !ok || host != "myhost" {
		t.Errorf("expected host %q, got %q (ok=%v)", "myhost", host, ok)
	}
	port, ok := doc.GetInt("server.port")
	if !ok || port != 9090 {
		t.Errorf("expected port %d, got %d (ok=%v)", 9090, port, ok)
	}

	// Missing key should be added.
	workers, ok := doc.GetInt("server.workers")
	if !ok || workers != 4 {
		t.Errorf("expected workers %d, got %d (ok=%v)", 4, workers, ok)
	}

	// Round-trip.
	out := doc.Bytes()
	doc2, err := Parse(out)
	if err != nil {
		t.Fatalf("round-trip parse failed: %v\noutput:\n%s", err, string(out))
	}
	w2, ok := doc2.GetInt("server.workers")
	if !ok || w2 != 4 {
		t.Errorf("round-trip: expected workers %d, got %d", 4, w2)
	}
}

// Test 2: MergeDefaults nested -- recursive merge with nested maps.
func TestMergeDefaults_Nested(t *testing.T) {
	doc := parseMergeDoc(t, `[server]
host = "myhost"

[server.database]
name = "prod"
`)
	defaults := map[string]any{
		"host": "localhost",
		"port": 8080,
		"database": map[string]any{
			"name":      "mydb",
			"pool_size": 10,
		},
	}
	err := doc.MergeDefaults("server", defaults)
	if err != nil {
		t.Fatalf("MergeDefaults returned error: %v", err)
	}

	// Existing keys unchanged.
	host, _ := doc.GetString("server.host")
	if host != "myhost" {
		t.Errorf("expected host %q, got %q", "myhost", host)
	}
	name, _ := doc.GetString("server.database.name")
	if name != "prod" {
		t.Errorf("expected database name %q, got %q", "prod", name)
	}

	// New keys added.
	port, ok := doc.GetInt("server.port")
	if !ok || port != 8080 {
		t.Errorf("expected port %d, got %d", 8080, port)
	}
	pool, ok := doc.GetInt("server.database.pool_size")
	if !ok || pool != 10 {
		t.Errorf("expected pool_size %d, got %d", 10, pool)
	}

	// Round-trip.
	out := doc.Bytes()
	if _, err := Parse(out); err != nil {
		t.Fatalf("round-trip parse failed: %v\noutput:\n%s", err, string(out))
	}
}

// Test 3: MergeDefaults with path -- merges within a table scope.
func TestMergeDefaults_WithPath(t *testing.T) {
	doc := parseMergeDoc(t, `[app]
name = "myapp"

[app.logging]
level = "info"
`)
	defaults := map[string]any{
		"level":  "debug",
		"format": "json",
	}
	err := doc.MergeDefaults("app.logging", defaults)
	if err != nil {
		t.Fatalf("MergeDefaults returned error: %v", err)
	}

	// Existing key unchanged.
	level, _ := doc.GetString("app.logging.level")
	if level != "info" {
		t.Errorf("expected level %q, got %q", "info", level)
	}

	// New key added.
	format, ok := doc.GetString("app.logging.format")
	if !ok || format != "json" {
		t.Errorf("expected format %q, got %q (ok=%v)", "json", format, ok)
	}
}

// Test 4: MergeDefaults empty target -- merge into empty document.
func TestMergeDefaults_EmptyTarget(t *testing.T) {
	doc := parseMergeDoc(t, ``)
	defaults := map[string]any{
		"host": "localhost",
		"port": 8080,
	}
	err := doc.MergeDefaults("", defaults)
	if err != nil {
		t.Fatalf("MergeDefaults returned error: %v", err)
	}

	host, ok := doc.GetString("host")
	if !ok || host != "localhost" {
		t.Errorf("expected host %q, got %q (ok=%v)", "localhost", host, ok)
	}
	port, ok := doc.GetInt("port")
	if !ok || port != 8080 {
		t.Errorf("expected port %d, got %d (ok=%v)", 8080, port, ok)
	}

	// Round-trip.
	out := doc.Bytes()
	if _, err := Parse(out); err != nil {
		t.Fatalf("round-trip parse failed: %v\noutput:\n%s", err, string(out))
	}
}

// Test 5: MergeDefaults no overwrite -- every existing key survives.
func TestMergeDefaults_NoOverwrite(t *testing.T) {
	doc := parseMergeDoc(t, `title = "existing"
count = 42
enabled = true
ratio = 3.14
`)
	defaults := map[string]any{
		"title":   "default",
		"count":   0,
		"enabled": false,
		"ratio":   1.0,
		"newkey":  "added",
	}
	err := doc.MergeDefaults("", defaults)
	if err != nil {
		t.Fatalf("MergeDefaults returned error: %v", err)
	}

	title, _ := doc.GetString("title")
	if title != "existing" {
		t.Errorf("title was overwritten: got %q", title)
	}
	count, _ := doc.GetInt("count")
	if count != 42 {
		t.Errorf("count was overwritten: got %d", count)
	}
	enabled, _ := doc.GetBool("enabled")
	if !enabled {
		t.Error("enabled was overwritten to false")
	}
	ratio, _ := doc.GetFloat("ratio")
	if ratio != 3.14 {
		t.Errorf("ratio was overwritten: got %f", ratio)
	}
	newkey, ok := doc.GetString("newkey")
	if !ok || newkey != "added" {
		t.Errorf("newkey not added: got %q (ok=%v)", newkey, ok)
	}
}

// Test 6: MergeDefaults arrays atomic -- existing array not merged element-by-element.
func TestMergeDefaults_ArraysAtomic(t *testing.T) {
	doc := parseMergeDoc(t, `tags = ["a", "b"]
`)
	defaults := map[string]any{
		"tags": []any{"x", "y", "z"},
	}
	err := doc.MergeDefaults("", defaults)
	if err != nil {
		t.Fatalf("MergeDefaults returned error: %v", err)
	}

	// The existing array should be unchanged.
	node, _ := doc.Lookup("tags")
	arr, ok := node.(*ArrayNode)
	if !ok {
		t.Fatalf("expected ArrayNode, got %T", node)
	}
	if len(arr.Elements) != 2 {
		t.Errorf("expected 2 elements, got %d", len(arr.Elements))
	}
	elem0, ok := arr.Elements[0].(*StringNode)
	if !ok || elem0.Val != "a" {
		t.Errorf("expected first element %q, got %v", "a", arr.Elements[0])
	}
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
	port, ok := target.GetInt("server.port")
	if !ok || port != 9090 {
		t.Errorf("expected port %d, got %d", 9090, port)
	}

	// New keys added.
	workers, ok := target.GetInt("server.workers")
	if !ok || workers != 4 {
		t.Errorf("expected workers %d, got %d (ok=%v)", 4, workers, ok)
	}

	level, ok := target.GetString("logging.level")
	if !ok || level != "debug" {
		t.Errorf("expected logging.level %q, got %q (ok=%v)", "debug", level, ok)
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

	workers, ok := target.GetInt("server.workers")
	if !ok || workers != 4 {
		t.Errorf("expected workers %d, got %d (ok=%v)", 4, workers, ok)
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
	version, ok := target.GetString("version")
	if !ok || version != "1.0.0" {
		t.Errorf("version not added: %q (ok=%v)", version, ok)
	}
	workers, ok := target.GetInt("server.workers")
	if !ok || workers != 4 {
		t.Errorf("server.workers not added: %d (ok=%v)", workers, ok)
	}
	debug, ok := target.GetBool("server.debug")
	if !ok || debug != false {
		t.Errorf("server.debug not added: %v (ok=%v)", debug, ok)
	}
	poolSize, ok := target.GetInt("database.pool_size")
	if !ok || poolSize != 5 {
		t.Errorf("database.pool_size not added: %d (ok=%v)", poolSize, ok)
	}
	timeout, ok := target.GetInt("database.timeout")
	if !ok || timeout != 30 {
		t.Errorf("database.timeout not added: %d (ok=%v)", timeout, ok)
	}
	level, ok := target.GetString("logging.level")
	if !ok || level != "info" {
		t.Errorf("logging.level not added: %q (ok=%v)", level, ok)
	}
	format, ok := target.GetString("logging.format")
	if !ok || format != "json" {
		t.Errorf("logging.format not added: %q (ok=%v)", format, ok)
	}

	// Round-trip.
	out := target.Bytes()
	if _, err := Parse(out); err != nil {
		t.Fatalf("round-trip parse failed: %v\noutput:\n%s", err, string(out))
	}
}
