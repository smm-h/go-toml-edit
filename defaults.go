package tomledit

// Default is one entry of the EnsureDefaults input: a full PATH in this
// package's path syntax, and the value to seed there when the document does
// not carry it.
//
// Path is a path, not a key: "server.host" reaches the "host" of the "server"
// table, and a key that carries a dot of its own is quoted, `server."host.name"`.
// Pair is the other way round -- one key, taken verbatim -- and the two are
// separate types so that neither grammar can be handed to the other by
// accident.
type Default struct {
	Path  string
	Value any
}

// EnsureDefaults seeds the paths the document does not already carry, in the
// order given, and returns the paths it added.
//
// A path the document carries in ANY spelling is left alone: a key written as
// a dotted key, inside a [header] table, or inside an inline table all count
// as present, and so does a table that only exists because a longer header
// implies it. Nothing is ever overwritten and no existing value, comment or
// spelling is touched.
//
// A missing intermediate table is created as a standard [header] table, never
// an inline one, so what the document ends up spelling depends only on the
// list and not on the order in which the intermediates happened to be needed:
// running the same list against the same document twice writes the same bytes.
//
// Partial application: the seeding stops at the first error, everything
// written before it stays written, and added names exactly those paths -- so a
// caller that must know what it changed reads them from the return value
// rather than from a diff. The default that failed wrote nothing at all, not
// even the tables leading to it, so added is exact rather than approximate.
func (d *Document) EnsureDefaults(defaults []Default) (added []string, err error) {
	for _, def := range defaults {
		if _, present := d.probe(def.Path); present {
			continue
		}
		if err := d.SetCreate(def.Path, def.Value); err != nil {
			return added, err
		}
		added = append(added, def.Path)
	}
	return added, nil
}
