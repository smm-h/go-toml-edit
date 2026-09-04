package tomledit

import (
	"bytes"
	"errors"
	"io/fs"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strconv"
)

// The write path's three seams: rendering, writing and renaming. They are
// variables so that the invariant tests can inject a failure at each step
// without a filesystem that fails on demand, and so that the round-trip check
// can be shown to catch a render that does not survive one. Nothing outside a
// test reassigns them.
var (
	renderForWrite = func(d *Document) []byte { return d.Bytes() }
	writeAll       = func(f *os.File, data []byte) error { _, err := f.Write(data); return err }
	renameOver     = os.Rename
)

// WriteFile renders the document and writes it to path, atomically, and only
// after the rendered bytes have proved they survive a round trip.
//
// The write is atomic in the sense a rename gives: the bytes go to a temporary
// file in the DESTINATION'S OWN DIRECTORY -- so the rename cannot cross a
// filesystem boundary -- and that file replaces the destination in one step. A
// failure anywhere before the rename leaves the destination exactly as it was
// and no temporary file behind. (A machine that loses power mid-write is the
// kernel's business, not this function's.)
//
// The round trip is checked before anything is written: the rendered bytes must
// parse, and re-rendering that parse must reproduce them byte for byte. A
// failure is an *Error of kind KindRoundTrip whose Offset field carries the
// byte at which the two disagree -- or, when the rendered bytes do not parse at
// all, the offset at which they stopped being TOML, with the parse error
// wrapped so errors.Is still reaches it. Nothing is written in either case.
//
// The destination's file mode is preserved when it already exists. A new file
// is created with mode 0o644, as an ordinary create would, so the process umask
// applies to it.
//
// A filesystem failure is reported as the underlying error -- an *fs.PathError,
// matchable with errors.Is -- and not as an *Error, on the same terms as
// ParseFile: nothing about the document is wrong, so there is nothing to
// diagnose about it.
func (d *Document) WriteFile(path string) error {
	out := renderForWrite(d)
	if err := verifyRoundTrip(out); err != nil {
		return err.atPath(path).inFile(path)
	}

	perm, existed, err := destinationMode(path)
	if err != nil {
		return err
	}

	f, name, err := createTemp(path, perm)
	if err != nil {
		return err
	}
	// From here on, every failure removes the temporary file: the destination
	// is only ever touched by the rename below.
	discard := func(err error) error {
		f.Close()
		os.Remove(name)
		return err
	}
	if err := writeAll(f, out); err != nil {
		return discard(err)
	}
	if err := f.Sync(); err != nil {
		return discard(err)
	}
	if err := f.Close(); err != nil {
		os.Remove(name)
		return err
	}
	if existed {
		// The temporary file was created with the destination's mode, but a
		// create applies the umask; the destination's own mode is the contract,
		// so it is set outright.
		if err := os.Chmod(name, perm); err != nil {
			os.Remove(name)
			return err
		}
	}
	if err := renameOver(name, path); err != nil {
		os.Remove(name)
		return err
	}
	return nil
}

// verifyRoundTrip reports whether rendered bytes survive a parse and a
// re-render, as the diagnostic they fail with.
func verifyRoundTrip(out []byte) *Error {
	back, err := Parse(out)
	if err != nil {
		diag := asDiagnostic(err)
		return newError(KindRoundTrip, "the rendered document does not parse: %s", diag.Message).
			at(diag.Pos).atOffset(diag.Pos.Offset).inSource(out).wrapping(err)
	}
	again := renderForWrite(back)
	if bytes.Equal(out, again) {
		return nil
	}
	off := firstDivergence(out, again)
	return newError(KindRoundTrip,
		"the rendered document does not survive a re-render: %d bytes written, %d bytes read back, first differing at byte %d",
		len(out), len(again), off).atOffset(off)
}

// firstDivergence returns the index of the first byte at which a and b differ,
// which for two byte slices where one is a prefix of the other is the length of
// the shorter.
func firstDivergence(a, b []byte) int {
	n := min(len(a), len(b))
	for i := 0; i < n; i++ {
		if a[i] != b[i] {
			return i
		}
	}
	return n
}

// destinationMode reports the mode the written file must end up with, and
// whether the destination already exists: an existing file keeps its own mode,
// and a new one is created as an ordinary create would make it.
func destinationMode(path string) (fs.FileMode, bool, error) {
	info, err := os.Stat(path)
	switch {
	case err == nil:
		return info.Mode().Perm(), true, nil
	case errors.Is(err, fs.ErrNotExist):
		return 0o644, false, nil
	default:
		return 0, false, err
	}
}

// createTemp creates the temporary file the rendered bytes are written to, in
// the destination's own directory and with the given mode. The name is a
// dot-prefixed sibling of the destination, so a directory read while a write is
// in flight does not show a stray visible file.
//
// os.CreateTemp is not usable here: it fixes the mode at 0o600, and the mode
// the file is CREATED with is what carries the umask contract.
func createTemp(path string, perm fs.FileMode) (*os.File, string, error) {
	dir, base := filepath.Split(path)
	if dir == "" {
		dir = "."
	}
	for attempt := 0; ; attempt++ {
		name := filepath.Join(dir, "."+base+".tmp"+strconv.FormatUint(uint64(rand.Uint32()), 36))
		f, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm)
		if err == nil {
			return f, name, nil
		}
		// A name already taken is the one failure worth another try; ten
		// thousand collisions in a row is a broken directory, not bad luck.
		if !errors.Is(err, fs.ErrExist) || attempt >= 10000 {
			return nil, "", err
		}
	}
}
