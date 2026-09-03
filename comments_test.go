package tomledit

import (
	"strings"
	"testing"
)

const commentsTestDocument = `[server]
host = "localhost"
port = 8080

[database]
name = "mydb"
`

func parseCommentsTestDoc(t *testing.T) *Document {
	t.Helper()
	doc, err := Parse([]byte(commentsTestDocument))
	if err != nil {
		t.Fatalf("failed to parse test document: %v", err)
	}
	return doc
}

// Test 1: Set inline comment on a key-value
func TestSetComment_KeyValue(t *testing.T) {
	doc := parseCommentsTestDoc(t)
	err := doc.SetComment("server.host", "the server hostname")
	if err != nil {
		t.Fatalf("SetComment returned error: %v", err)
	}
	out := string(doc.Bytes())
	if !strings.Contains(out, "# the server hostname") {
		t.Errorf("expected inline comment in output, got:\n%s", out)
	}
}

// Test 2: Set leading comments on a key-value
func TestSetLeadingComments_KeyValue(t *testing.T) {
	doc := parseCommentsTestDoc(t)
	err := doc.SetLeadingComments("server.port", []string{"Port configuration", "Change as needed"})
	if err != nil {
		t.Fatalf("SetLeadingComments returned error: %v", err)
	}
	out := string(doc.Bytes())
	if !strings.Contains(out, "# Port configuration") {
		t.Errorf("expected leading comment line 1 in output, got:\n%s", out)
	}
	if !strings.Contains(out, "# Change as needed") {
		t.Errorf("expected leading comment line 2 in output, got:\n%s", out)
	}
}

// Test 3: Set comment on a table header
func TestSetComment_TableHeader(t *testing.T) {
	doc := parseCommentsTestDoc(t)
	err := doc.SetComment("database", "DB settings")
	if err != nil {
		t.Fatalf("SetComment returned error: %v", err)
	}
	out := string(doc.Bytes())
	if !strings.Contains(out, "[database] # DB settings") {
		t.Errorf("expected inline comment on table header, got:\n%s", out)
	}
}

// Test 4: Remove comment (empty string)
func TestSetComment_Remove(t *testing.T) {
	// First set a comment, then remove it
	doc := parseCommentsTestDoc(t)
	err := doc.SetComment("server.host", "some comment")
	if err != nil {
		t.Fatalf("SetComment (set) returned error: %v", err)
	}
	out1 := string(doc.Bytes())
	if !strings.Contains(out1, "# some comment") {
		t.Fatalf("comment was not set, got:\n%s", out1)
	}

	err = doc.SetComment("server.host", "")
	if err != nil {
		t.Fatalf("SetComment (remove) returned error: %v", err)
	}
	out2 := string(doc.Bytes())
	if strings.Contains(out2, "# some comment") {
		t.Errorf("comment was not removed, got:\n%s", out2)
	}
}

// Test 5: Error on non-existent path
func TestSetComment_NonExistent(t *testing.T) {
	doc := parseCommentsTestDoc(t)
	err := doc.SetComment("nonexistent.key", "comment")
	if err == nil {
		t.Fatal("expected error for non-existent path, got nil")
	}
}

// Test 6: Round-trip: set comment, serialize, verify comment appears in output
func TestSetComment_RoundTrip(t *testing.T) {
	doc := parseCommentsTestDoc(t)
	err := doc.SetComment("server.host", "important setting")
	if err != nil {
		t.Fatalf("SetComment returned error: %v", err)
	}
	out := doc.Bytes()
	doc2, err := Parse(out)
	if err != nil {
		t.Fatalf("round-trip parse failed: %v\noutput was:\n%s", err, string(out))
	}
	// The re-parsed document should still have the host value
	val, ok := doc2.GetString("server.host")
	if !ok || val != "localhost" {
		t.Errorf("round-trip: expected host=\"localhost\", got %q (ok=%v)", val, ok)
	}
	// And the comment should be in the serialized output
	out2 := string(doc2.Bytes())
	if !strings.Contains(out2, "# important setting") {
		t.Errorf("round-trip: comment lost after re-parse, got:\n%s", out2)
	}
}

// Test 7: Set comment then Get still works (comments don't affect values)
func TestSetComment_GetStillWorks(t *testing.T) {
	doc := parseCommentsTestDoc(t)
	err := doc.SetComment("server.port", "HTTP port")
	if err != nil {
		t.Fatalf("SetComment returned error: %v", err)
	}
	val, ok := doc.GetInt("server.port")
	if !ok || val != 8080 {
		t.Errorf("expected port=8080 after SetComment, got %d (ok=%v)", val, ok)
	}

	err = doc.SetLeadingComments("database.name", []string{"Database name"})
	if err != nil {
		t.Fatalf("SetLeadingComments returned error: %v", err)
	}
	name, ok := doc.GetString("database.name")
	if !ok || name != "mydb" {
		t.Errorf("expected name=\"mydb\" after SetLeadingComments, got %q (ok=%v)", name, ok)
	}
}
