package tomledit

import (
	"testing"
	"time"
)

// The accessor families read the conversion table in this phase, so two rows
// the getters used to refuse start answering: an integer a float target holds
// exactly, and the local date-time flavors a time.Time target accepts.

// Fails if the float accessors stop accepting an integer the target holds
// exactly -- the conversion table's integer-into-float row.
func TestAccessorWidening_ExactIntegerReadsAsFloat(t *testing.T) {
	doc, err := Parse([]byte("val = 42\n"))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	got, ok := doc.GetFloat("val")
	if !ok {
		t.Errorf("GetFloat on an exactly representable integer refused it")
	}
	if got != 42 {
		t.Errorf("GetFloat read %v, want 42", got)
	}

	cur, ok := doc.Key("val").Float()
	if !ok {
		t.Errorf("Cursor.Float on an exactly representable integer refused it")
	}
	if cur != 42 {
		t.Errorf("Cursor.Float read %v, want 42", cur)
	}
}

// Fails if the time accessors stop following the conversion table's time.Time
// rows: a local date-time and a local date convert, because the declared
// target expresses the intent.
func TestAccessorWidening_LocalFlavorsReadAsTime(t *testing.T) {
	doc, err := Parse([]byte("dt = 1979-05-27T07:32:00\nd = 1979-05-27\n"))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	got, ok := doc.GetTime("dt")
	if !ok {
		t.Errorf("GetTime on a local date-time refused it")
	}
	if want := time.Date(1979, 5, 27, 7, 32, 0, 0, time.UTC); !got.Equal(want) {
		t.Errorf("GetTime read %v, want %v", got, want)
	}

	got, ok = doc.GetTime("d")
	if !ok {
		t.Errorf("GetTime on a local date refused it")
	}
	if want := time.Date(1979, 5, 27, 0, 0, 0, 0, time.UTC); !got.Equal(want) {
		t.Errorf("GetTime read %v, want %v", got, want)
	}
}
