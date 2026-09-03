package tomledit

import (
	"encoding"
	"fmt"
	"reflect"
	"strings"
	"time"
)

// Unmarshaler is implemented by types that can unmarshal themselves from a TOML
// Node. The node passed to UnmarshalTOML is the raw AST node (e.g. a StringNode,
// IntegerNode, or TableNode), allowing custom decoding logic.
type Unmarshaler interface {
	UnmarshalTOML(node Node) error
}

// Unmarshal parses TOML data and decodes it into v.
//
// v must be a non-nil pointer to a struct or map[string]any. Struct fields are
// matched by their "toml" tag, then by exact field name, then by case-insensitive
// name. Unknown TOML keys are silently ignored. Types implementing Unmarshaler
// or encoding.TextUnmarshaler are handled automatically.
func Unmarshal(data []byte, v any) error {
	doc, err := Parse(data)
	if err != nil {
		return err
	}
	return doc.Decode(v)
}

// Decode decodes the document's content into v.
// v must be a non-nil pointer to a struct or map[string]any. Unlike Unmarshal,
// Decode operates on an already-parsed Document, which is useful when you
// need both the AST (for editing) and the decoded values.
func (d *Document) Decode(v any) error {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Pointer || rv.IsNil() {
		return fmt.Errorf("toml: Decode requires a non-nil pointer, got %T", v)
	}
	dec := &decoder{}
	return dec.decodeDocument(d, rv.Elem())
}

// decoder holds state during decoding.
type decoder struct{}

func (dec *decoder) decodeDocument(doc *Document, rv reflect.Value) error {
	rv = dec.indirect(rv, false)
	switch rv.Kind() {
	case reflect.Struct:
		return dec.decodeDocumentToStruct(doc, rv)
	case reflect.Map:
		return dec.decodeDocumentToMap(doc, rv)
	case reflect.Interface:
		m := make(map[string]any)
		mapVal := reflect.ValueOf(&m).Elem()
		if err := dec.decodeDocumentToMap(doc, mapVal); err != nil {
			return err
		}
		rv.Set(mapVal)
		return nil
	default:
		return fmt.Errorf("toml: cannot decode document into %s", rv.Type())
	}
}

// decodeDocumentToStruct walks the document's top-level Children and dispatches each
// into the target struct.
func (dec *decoder) decodeDocumentToStruct(doc *Document, rv reflect.Value) error {
	fm := newFieldMapping(rv.Type())

	// Collect top-level array-table paths so we can skip their sub-tables.
	arrayTablePaths := collectArrayTablePaths(doc)

	for _, child := range doc.Children {
		switch n := child.(type) {
		case *CommentNode:
			continue
		case *KeyValueNode:
			if err := dec.decodeKVIntoStruct(n, fm, rv, ""); err != nil {
				return err
			}
		case *TableNode:
			// Skip TableNode entries that are sub-tables of an array-of-tables.
			// These are handled by decodeArrayTableSubTables when processing the parent.
			if isSubTableOfArrayTable(n.KeyPath, arrayTablePaths) {
				continue
			}
			if err := dec.decodeTableNodeIntoStruct(doc, n, fm, rv); err != nil {
				return err
			}
		case *ArrayTableNode:
			// Skip sub-array-tables (multi-component KeyPath); they are handled
			// by decodeArrayTableSubTables when processing their parent.
			if len(n.KeyPath) > 1 {
				continue
			}
			if err := dec.decodeArrayTableNodeIntoStruct(doc, n, fm, rv); err != nil {
				return err
			}
		}
	}
	return nil
}

// decodeDocumentToMap walks the document's top-level Children and adds each into the map.
func (dec *decoder) decodeDocumentToMap(doc *Document, rv reflect.Value) error {
	if rv.IsNil() {
		rv.Set(reflect.MakeMap(rv.Type()))
	}
	arraySlices := map[string][]any{}

	// Collect top-level array-table paths so we can skip their sub-tables.
	arrayTablePaths := collectArrayTablePaths(doc)

	for _, child := range doc.Children {
		switch n := child.(type) {
		case *CommentNode:
			continue
		case *KeyValueNode:
			if err := dec.decodeKVIntoMap(n, rv, ""); err != nil {
				return err
			}
		case *TableNode:
			// Skip TableNode entries that are sub-tables of an array-of-tables.
			// These are handled by decodeArrayTableSubTablesIntoMap.
			if isSubTableOfArrayTable(n.KeyPath, arrayTablePaths) {
				continue
			}
			if err := dec.decodeTableNodeIntoMap(doc, n, rv); err != nil {
				return err
			}
		case *ArrayTableNode:
			// Skip sub-array-tables (multi-component KeyPath); they are handled
			// by decodeArrayTableSubTablesIntoMap when processing their parent.
			if len(n.KeyPath) > 1 {
				continue
			}
			if err := dec.decodeArrayTableNodeIntoMap(doc, n, rv, arraySlices); err != nil {
				return err
			}
		}
	}
	return nil
}

// --- struct field mapping ---

// structField stores the index path for navigating to a field, possibly through embeddings.
type structField struct {
	index []int // field index path (e.g. [0] or [1,3] for embedded)
}

// fieldMapping maps TOML key names to struct fields.
type fieldMapping struct {
	fields map[string]structField
}

func newFieldMapping(t reflect.Type) *fieldMapping {
	fm := &fieldMapping{fields: make(map[string]structField)}
	lowerFallback := make(map[string]structField)
	collectFields(t, nil, fm.fields, lowerFallback)
	// Add case-insensitive fallbacks that don't shadow exact matches.
	for name, sf := range lowerFallback {
		if _, exists := fm.fields[name]; !exists {
			fm.fields[name] = sf
		}
	}
	return fm
}

// collectFields recursively collects struct fields, handling embedded structs.
func collectFields(t reflect.Type, indexPrefix []int, exact map[string]structField, lower map[string]structField) {
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}

		idx := append(append([]int(nil), indexPrefix...), i)

		tag := f.Tag.Get("toml")
		if tag == "-" {
			continue
		}
		tagName, _ := parseTag(tag)

		if f.Anonymous && tagName == "" {
			ft := f.Type
			if ft.Kind() == reflect.Pointer {
				ft = ft.Elem()
			}
			if ft.Kind() == reflect.Struct {
				// Promote embedded struct fields.
				collectFields(ft, idx, exact, lower)
				continue
			}
		}

		name := tagName
		if name == "" {
			name = f.Name
		}

		sf := structField{index: idx}
		if _, exists := exact[name]; !exists {
			exact[name] = sf
		}
		lo := strings.ToLower(name)
		if _, exists := exact[lo]; !exists {
			if _, exists2 := lower[lo]; !exists2 {
				lower[lo] = sf
			}
		}
	}
}

// lookup finds the struct field for a given TOML key name.
// It tries an exact match first, then falls back to case-insensitive matching.
func (fm *fieldMapping) lookup(name string) (structField, bool) {
	sf, ok := fm.fields[name]
	if ok {
		return sf, true
	}
	// Case-insensitive fallback: the map contains lowercase keys as fallbacks.
	sf, ok = fm.fields[strings.ToLower(name)]
	return sf, ok
}

// fieldByIndex navigates to the reflect.Value for a field given its index path,
// allocating nil pointer embeddings along the way.
func fieldByIndex(rv reflect.Value, idx []int) reflect.Value {
	for _, i := range idx {
		if rv.Kind() == reflect.Pointer {
			if rv.IsNil() {
				rv.Set(reflect.New(rv.Type().Elem()))
			}
			rv = rv.Elem()
		}
		rv = rv.Field(i)
	}
	return rv
}

// --- KV into struct ---

func (dec *decoder) decodeKVIntoStruct(kv *KeyValueNode, fm *fieldMapping, rv reflect.Value, pathPrefix string) error {
	parts := kv.Key.Parts
	if len(parts) == 0 {
		return nil
	}

	current := rv
	currentFM := fm
	currentPath := pathPrefix

	for i, part := range parts {
		currentPath = appendPath(currentPath, part)

		sf, ok := currentFM.lookup(part)
		if !ok {
			return nil // extra key, silently ignore
		}

		fv := fieldByIndex(current, sf.index)
		fv = dec.indirect(fv, false)

		if i < len(parts)-1 {
			// Intermediate part of dotted key.
			switch fv.Kind() {
			case reflect.Struct:
				current = fv
				currentFM = newFieldMapping(fv.Type())
			case reflect.Map:
				return dec.decodeDottedKVIntoMap(kv, parts[i+1:], fv, currentPath)
			case reflect.Interface:
				if fv.IsNil() {
					m := make(map[string]any)
					fv.Set(reflect.ValueOf(m))
				}
				inner := fv.Elem()
				if inner.Kind() == reflect.Map {
					return dec.decodeDottedKVIntoMap(kv, parts[i+1:], inner, currentPath)
				}
				return fmt.Errorf("toml: cannot decode dotted key into %s at %q", fv.Type(), currentPath)
			default:
				return fmt.Errorf("toml: cannot decode dotted key into %s at %q", fv.Type(), currentPath)
			}
		} else {
			return dec.decodeValue(kv.Val, fv, currentPath)
		}
	}
	return nil
}

// --- KV into map ---

func (dec *decoder) decodeKVIntoMap(kv *KeyValueNode, rv reflect.Value, pathPrefix string) error {
	parts := kv.Key.Parts
	if len(parts) == 0 {
		return nil
	}
	if rv.Type().Key().Kind() != reflect.String {
		return fmt.Errorf("toml: cannot decode into map with non-string key type %s", rv.Type().Key())
	}
	if rv.IsNil() {
		rv.Set(reflect.MakeMap(rv.Type()))
	}

	current := rv
	for i, part := range parts {
		currentPath := appendPath(pathPrefix, part)
		keyVal := reflect.ValueOf(part)
		if i < len(parts)-1 {
			current = ensureSubMap(current, keyVal)
		} else {
			elemType := rv.Type().Elem()
			val := reflect.New(elemType).Elem()
			if err := dec.decodeValue(kv.Val, val, currentPath); err != nil {
				return err
			}
			current.SetMapIndex(keyVal, val)
		}
	}
	return nil
}

// decodeDottedKVIntoMap handles the tail of a dotted key when entering a map.
func (dec *decoder) decodeDottedKVIntoMap(kv *KeyValueNode, remaining []string, rv reflect.Value, pathPrefix string) error {
	if rv.IsNil() {
		rv.Set(reflect.MakeMap(rv.Type()))
	}
	current := rv
	for i, part := range remaining {
		keyVal := reflect.ValueOf(part)
		if i < len(remaining)-1 {
			current = ensureSubMap(current, keyVal)
		} else {
			val, err := dec.decodeToInterface(kv.Val)
			if err != nil {
				return err
			}
			current.SetMapIndex(keyVal, reflect.ValueOf(val))
		}
	}
	return nil
}

// --- Table node into struct ---

func (dec *decoder) decodeTableNodeIntoStruct(doc *Document, tbl *TableNode, fm *fieldMapping, rv reflect.Value) error {
	if len(tbl.KeyPath) == 0 {
		return nil
	}

	current := rv
	currentFM := fm
	path := ""

	for _, part := range tbl.KeyPath {
		path = appendPath(path, part)

		sf, ok := currentFM.lookup(part)
		if !ok {
			return nil
		}
		fv := fieldByIndex(current, sf.index)
		fv = dec.indirect(fv, false)

		switch fv.Kind() {
		case reflect.Struct:
			current = fv
			currentFM = newFieldMapping(fv.Type())
		case reflect.Slice:
			// Navigate into the last element (for sub-tables of array-of-tables).
			if fv.Len() == 0 {
				return nil
			}
			elem := fv.Index(fv.Len() - 1)
			elem = dec.indirect(elem, false)
			if elem.Kind() != reflect.Struct {
				return fmt.Errorf("toml: cannot decode Table through non-struct slice element at %q", path)
			}
			current = elem
			currentFM = newFieldMapping(elem.Type())
		case reflect.Map:
			return dec.decodeTableChildrenIntoMap(tbl, fv)
		case reflect.Interface:
			if fv.IsNil() {
				m := make(map[string]any)
				fv.Set(reflect.ValueOf(m))
			}
			inner := fv.Elem()
			if inner.Kind() == reflect.Map {
				return dec.decodeTableChildrenIntoMap(tbl, inner)
			}
			return fmt.Errorf("toml: cannot decode table into %s at %q", fv.Type(), path)
		default:
			return fmt.Errorf("toml: cannot decode Table into %s at %q", fv.Type(), path)
		}
	}

	return dec.decodeChildrenIntoStruct(tbl.Children, current, strings.Join(tbl.KeyPath, "."))
}

// --- Table node into map ---

func (dec *decoder) decodeTableNodeIntoMap(doc *Document, tbl *TableNode, rv reflect.Value) error {
	if rv.IsNil() {
		rv.Set(reflect.MakeMap(rv.Type()))
	}
	current := rv
	for _, part := range tbl.KeyPath {
		current = ensureSubMap(current, reflect.ValueOf(part))
	}
	pathPrefix := strings.Join(tbl.KeyPath, ".")
	for _, child := range tbl.Children {
		if kv, ok := child.(*KeyValueNode); ok {
			if err := dec.decodeKVIntoMap(kv, current, pathPrefix); err != nil {
				return err
			}
		}
	}
	return nil
}

// decodeTableChildrenIntoMap decodes a table's children into a map.
func (dec *decoder) decodeTableChildrenIntoMap(tbl *TableNode, rv reflect.Value) error {
	if rv.IsNil() {
		rv.Set(reflect.MakeMap(rv.Type()))
	}
	pathPrefix := strings.Join(tbl.KeyPath, ".")
	for _, child := range tbl.Children {
		if kv, ok := child.(*KeyValueNode); ok {
			if err := dec.decodeKVIntoMap(kv, rv, pathPrefix); err != nil {
				return err
			}
		}
	}
	return nil
}

// --- Array table into struct ---

func (dec *decoder) decodeArrayTableNodeIntoStruct(doc *Document, atbl *ArrayTableNode, fm *fieldMapping, rv reflect.Value) error {
	if len(atbl.KeyPath) == 0 {
		return nil
	}

	current := rv
	currentFM := fm
	path := ""

	for i, part := range atbl.KeyPath {
		path = appendPath(path, part)

		sf, ok := currentFM.lookup(part)
		if !ok {
			return nil
		}
		fv := fieldByIndex(current, sf.index)
		fv = dec.indirect(fv, false)

		if i < len(atbl.KeyPath)-1 {
			switch fv.Kind() {
			case reflect.Struct:
				current = fv
				currentFM = newFieldMapping(fv.Type())
			case reflect.Slice:
				if fv.Len() == 0 {
					return nil
				}
				elem := fv.Index(fv.Len() - 1)
				elem = dec.indirect(elem, false)
				if elem.Kind() != reflect.Struct {
					return fmt.Errorf("toml: cannot decode array table into non-struct slice element at %q", path)
				}
				current = elem
				currentFM = newFieldMapping(elem.Type())
			default:
				return fmt.Errorf("toml: cannot decode array table path through %s at %q", fv.Type(), path)
			}
		} else {
			if fv.Kind() != reflect.Slice {
				return fmt.Errorf("toml: cannot decode ArrayTable into %s at %q (expected slice)", fv.Type(), path)
			}
			elemType := fv.Type().Elem()
			elemPtr := elemType.Kind() == reflect.Pointer
			if elemPtr {
				elemType = elemType.Elem()
			}
			newElem := reflect.New(elemType).Elem()

			if err := dec.decodeChildrenIntoValue(atbl.Children, newElem, path); err != nil {
				return err
			}
			if err := dec.decodeArrayTableSubTables(doc, atbl, newElem, path); err != nil {
				return err
			}

			if elemPtr {
				ptr := reflect.New(elemType)
				ptr.Elem().Set(newElem)
				fv.Set(reflect.Append(fv, ptr))
			} else {
				fv.Set(reflect.Append(fv, newElem))
			}
		}
	}
	return nil
}

// decodeArrayTableSubTables finds sub-tables scoped to a specific array table entry and decodes them.
func (dec *decoder) decodeArrayTableSubTables(doc *Document, atbl *ArrayTableNode, rv reflect.Value, pathPrefix string) error {
	scopeIdx := -1
	for i, child := range doc.Children {
		if child == atbl {
			scopeIdx = i
			break
		}
	}
	if scopeIdx == -1 {
		return nil
	}

	rv = dec.indirect(rv, false)
	if rv.Kind() != reflect.Struct {
		return nil
	}
	fm := newFieldMapping(rv.Type())

	for i := scopeIdx + 1; i < len(doc.Children); i++ {
		child := doc.Children[i]
		if at, ok := child.(*ArrayTableNode); ok {
			if pathsEqual(at.KeyPath, atbl.KeyPath) {
				break
			}
		}

		if tbl, ok := child.(*TableNode); ok {
			if hasPrefix(tbl.KeyPath, atbl.KeyPath) && len(tbl.KeyPath) > len(atbl.KeyPath) {
				subPath := tbl.KeyPath[len(atbl.KeyPath):]
				if err := dec.decodeSubTableIntoStruct(doc, tbl, subPath, fm, rv, pathPrefix); err != nil {
					return err
				}
			}
		}

		if at, ok := child.(*ArrayTableNode); ok {
			if hasPrefix(at.KeyPath, atbl.KeyPath) && len(at.KeyPath) > len(atbl.KeyPath) {
				subPath := at.KeyPath[len(atbl.KeyPath):]
				if err := dec.decodeSubArrayTableIntoStruct(doc, at, subPath, fm, rv, pathPrefix); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (dec *decoder) decodeSubTableIntoStruct(doc *Document, tbl *TableNode, relPath []string, fm *fieldMapping, rv reflect.Value, pathPrefix string) error {
	current := rv
	currentFM := fm
	path := pathPrefix

	for _, part := range relPath {
		path = appendPath(path, part)
		sf, ok := currentFM.lookup(part)
		if !ok {
			return nil
		}
		fv := fieldByIndex(current, sf.index)
		fv = dec.indirect(fv, false)

		switch fv.Kind() {
		case reflect.Struct:
			current = fv
			currentFM = newFieldMapping(fv.Type())
		case reflect.Map:
			return dec.decodeTableChildrenIntoMap(tbl, fv)
		default:
			return fmt.Errorf("toml: cannot decode Table into %s at %q", fv.Type(), path)
		}
	}
	return dec.decodeChildrenIntoStruct(tbl.Children, current, path)
}

func (dec *decoder) decodeSubArrayTableIntoStruct(doc *Document, atbl *ArrayTableNode, relPath []string, fm *fieldMapping, rv reflect.Value, pathPrefix string) error {
	current := rv
	currentFM := fm
	path := pathPrefix

	for i, part := range relPath {
		path = appendPath(path, part)
		sf, ok := currentFM.lookup(part)
		if !ok {
			return nil
		}
		fv := fieldByIndex(current, sf.index)
		fv = dec.indirect(fv, false)

		if i < len(relPath)-1 {
			switch fv.Kind() {
			case reflect.Struct:
				current = fv
				currentFM = newFieldMapping(fv.Type())
			case reflect.Slice:
				if fv.Len() == 0 {
					return nil
				}
				elem := fv.Index(fv.Len() - 1)
				elem = dec.indirect(elem, false)
				if elem.Kind() != reflect.Struct {
					return fmt.Errorf("toml: cannot navigate through non-struct slice at %q", path)
				}
				current = elem
				currentFM = newFieldMapping(elem.Type())
			default:
				return fmt.Errorf("toml: cannot navigate through %s at %q", fv.Type(), path)
			}
		} else {
			if fv.Kind() != reflect.Slice {
				return fmt.Errorf("toml: cannot decode ArrayTable into %s at %q", fv.Type(), path)
			}
			elemType := fv.Type().Elem()
			elemPtr := elemType.Kind() == reflect.Pointer
			if elemPtr {
				elemType = elemType.Elem()
			}
			newElem := reflect.New(elemType).Elem()
			if err := dec.decodeChildrenIntoValue(atbl.Children, newElem, path); err != nil {
				return err
			}
			if err := dec.decodeArrayTableSubTables(doc, atbl, newElem, path); err != nil {
				return err
			}
			if elemPtr {
				ptr := reflect.New(elemType)
				ptr.Elem().Set(newElem)
				fv.Set(reflect.Append(fv, ptr))
			} else {
				fv.Set(reflect.Append(fv, newElem))
			}
		}
	}
	return nil
}

// --- Array table into map ---

func (dec *decoder) decodeArrayTableNodeIntoMap(doc *Document, atbl *ArrayTableNode, rv reflect.Value, slices map[string][]any) error {
	if rv.IsNil() {
		rv.Set(reflect.MakeMap(rv.Type()))
	}

	current := rv
	for i, part := range atbl.KeyPath {
		keyVal := reflect.ValueOf(part)
		if i < len(atbl.KeyPath)-1 {
			current = ensureSubMap(current, keyVal)
		} else {
			pathKey := strings.Join(atbl.KeyPath, ".")
			entryMap := make(map[string]any)
			for _, child := range atbl.Children {
				if kv, ok := child.(*KeyValueNode); ok {
					val, err := dec.decodeToInterface(kv.Val)
					if err != nil {
						return err
					}
					setNestedMapValue(entryMap, kv.Key.Parts, val)
				}
			}
			dec.decodeArrayTableSubTablesIntoMap(doc, atbl, entryMap)

			slices[pathKey] = append(slices[pathKey], entryMap)
			current.SetMapIndex(keyVal, reflect.ValueOf(slices[pathKey]))
		}
	}
	return nil
}

func (dec *decoder) decodeArrayTableSubTablesIntoMap(doc *Document, atbl *ArrayTableNode, entryMap map[string]any) {
	scopeIdx := -1
	for i, child := range doc.Children {
		if child == atbl {
			scopeIdx = i
			break
		}
	}
	if scopeIdx == -1 {
		return
	}
	for i := scopeIdx + 1; i < len(doc.Children); i++ {
		child := doc.Children[i]
		if at, ok := child.(*ArrayTableNode); ok {
			if pathsEqual(at.KeyPath, atbl.KeyPath) {
				break
			}
		}
		if tbl, ok := child.(*TableNode); ok {
			if hasPrefix(tbl.KeyPath, atbl.KeyPath) && len(tbl.KeyPath) > len(atbl.KeyPath) {
				subPath := tbl.KeyPath[len(atbl.KeyPath):]
				current := entryMap
				for j, part := range subPath {
					if j < len(subPath)-1 {
						sub, ok := current[part]
						if !ok {
							sub = make(map[string]any)
							current[part] = sub
						}
						if m, ok := sub.(map[string]any); ok {
							current = m
						}
					} else {
						sub := make(map[string]any)
						if existing, ok := current[part]; ok {
							if m, ok := existing.(map[string]any); ok {
								sub = m
							}
						}
						current[part] = sub
						for _, tc := range tbl.Children {
							if kv, ok := tc.(*KeyValueNode); ok {
								val, err := dec.decodeToInterface(kv.Val)
								if err == nil {
									setNestedMapValue(sub, kv.Key.Parts, val)
								}
							}
						}
					}
				}
			}
		}
	}
}

// --- decode children into struct/value ---

func (dec *decoder) decodeChildrenIntoStruct(children []Node, rv reflect.Value, pathPrefix string) error {
	fm := newFieldMapping(rv.Type())
	for _, child := range children {
		if kv, ok := child.(*KeyValueNode); ok {
			if err := dec.decodeKVIntoStruct(kv, fm, rv, pathPrefix); err != nil {
				return err
			}
		}
	}
	return nil
}

func (dec *decoder) decodeChildrenIntoValue(children []Node, rv reflect.Value, pathPrefix string) error {
	rv = dec.indirect(rv, false)

	// Check for Unmarshaler
	if rv.CanAddr() {
		if u, ok := rv.Addr().Interface().(Unmarshaler); ok {
			tbl := &TableNode{Children: children}
			return u.UnmarshalTOML(tbl)
		}
	}

	switch rv.Kind() {
	case reflect.Struct:
		return dec.decodeChildrenIntoStruct(children, rv, pathPrefix)
	case reflect.Map:
		if rv.IsNil() {
			rv.Set(reflect.MakeMap(rv.Type()))
		}
		for _, child := range children {
			if kv, ok := child.(*KeyValueNode); ok {
				if err := dec.decodeKVIntoMap(kv, rv, pathPrefix); err != nil {
					return err
				}
			}
		}
		return nil
	case reflect.Interface:
		m := make(map[string]any)
		mapVal := reflect.ValueOf(&m).Elem()
		for _, child := range children {
			if kv, ok := child.(*KeyValueNode); ok {
				if err := dec.decodeKVIntoMap(kv, mapVal, pathPrefix); err != nil {
					return err
				}
			}
		}
		rv.Set(reflect.ValueOf(m))
		return nil
	default:
		return fmt.Errorf("toml: cannot decode table into %s at %q", rv.Type(), pathPrefix)
	}
}

// --- value decoding ---

func (dec *decoder) decodeValue(node Node, rv reflect.Value, path string) error {
	rv = dec.indirect(rv, false)

	// Check for Unmarshaler
	if rv.CanAddr() {
		if u, ok := rv.Addr().Interface().(Unmarshaler); ok {
			return u.UnmarshalTOML(node)
		}
	}

	// Check for encoding.TextUnmarshaler on string nodes
	if node.Type() == NodeString {
		if rv.CanAddr() {
			if tu, ok := rv.Addr().Interface().(encoding.TextUnmarshaler); ok {
				return tu.UnmarshalText([]byte(node.(*StringNode).Val))
			}
		}
	}

	// Handle interface{} targets
	if rv.Kind() == reflect.Interface {
		val, err := dec.decodeToInterface(node)
		if err != nil {
			return err
		}
		if val != nil {
			rv.Set(reflect.ValueOf(val))
		}
		return nil
	}

	switch n := node.(type) {
	case *StringNode:
		return dec.decodeString(n, rv, path)
	case *IntegerNode:
		return dec.decodeInteger(n, rv, path)
	case *FloatNode:
		return dec.decodeFloat(n, rv, path)
	case *BooleanNode:
		return dec.decodeBoolean(n, rv, path)
	case *DateTimeNode:
		return dec.decodeDateTime(n, rv, path)
	case *LocalDateTimeNode:
		return dec.decodeLocalDateTime(n, rv, path)
	case *LocalDateNode:
		return dec.decodeLocalDate(n, rv, path)
	case *LocalTimeNode:
		return dec.decodeLocalTime(n, rv, path)
	case *ArrayNode:
		return dec.decodeArray(n, rv, path)
	case *InlineTableNode:
		return dec.decodeInlineTable(n, rv, path)
	default:
		return fmt.Errorf("toml: unexpected node type %s at %q", node.Type(), path)
	}
}

func (dec *decoder) decodeString(n *StringNode, rv reflect.Value, path string) error {
	if rv.Kind() != reflect.String {
		return fmt.Errorf("toml: cannot decode String into %s at %q", rv.Type(), path)
	}
	rv.SetString(n.Val)
	return nil
}

func (dec *decoder) decodeInteger(n *IntegerNode, rv reflect.Value, path string) error {
	switch rv.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if rv.OverflowInt(n.Val) {
			return fmt.Errorf("toml: integer %d overflows %s at %q", n.Val, rv.Type(), path)
		}
		rv.SetInt(n.Val)
		return nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if n.Val < 0 {
			return fmt.Errorf("toml: negative integer %d cannot be stored in %s at %q", n.Val, rv.Type(), path)
		}
		uval := uint64(n.Val)
		if rv.OverflowUint(uval) {
			return fmt.Errorf("toml: integer %d overflows %s at %q", n.Val, rv.Type(), path)
		}
		rv.SetUint(uval)
		return nil
	case reflect.Float32, reflect.Float64:
		rv.SetFloat(float64(n.Val))
		return nil
	default:
		return fmt.Errorf("toml: cannot decode Integer into %s at %q", rv.Type(), path)
	}
}

func (dec *decoder) decodeFloat(n *FloatNode, rv reflect.Value, path string) error {
	switch rv.Kind() {
	case reflect.Float32, reflect.Float64:
		if rv.OverflowFloat(n.Val) {
			return fmt.Errorf("toml: float %g overflows %s at %q", n.Val, rv.Type(), path)
		}
		rv.SetFloat(n.Val)
		return nil
	default:
		return fmt.Errorf("toml: cannot decode Float into %s at %q", rv.Type(), path)
	}
}

func (dec *decoder) decodeBoolean(n *BooleanNode, rv reflect.Value, path string) error {
	if rv.Kind() != reflect.Bool {
		return fmt.Errorf("toml: cannot decode Boolean into %s at %q", rv.Type(), path)
	}
	rv.SetBool(n.Val)
	return nil
}

var timeType = reflect.TypeOf(time.Time{})

func (dec *decoder) decodeDateTime(n *DateTimeNode, rv reflect.Value, path string) error {
	if rv.Type() == timeType {
		rv.Set(reflect.ValueOf(n.Val))
		return nil
	}
	return fmt.Errorf("toml: cannot decode DateTime into %s at %q", rv.Type(), path)
}

func (dec *decoder) decodeLocalDateTime(n *LocalDateTimeNode, rv reflect.Value, path string) error {
	if rv.Type() == reflect.TypeOf(LocalDateTime{}) {
		rv.Set(reflect.ValueOf(n.Val))
		return nil
	}
	if rv.Type() == timeType {
		t := time.Date(n.Val.Year, time.Month(n.Val.Month), n.Val.Day,
			n.Val.Hour, n.Val.Minute, n.Val.Second, n.Val.Nanosecond, time.UTC)
		rv.Set(reflect.ValueOf(t))
		return nil
	}
	return fmt.Errorf("toml: cannot decode LocalDateTime into %s at %q", rv.Type(), path)
}

func (dec *decoder) decodeLocalDate(n *LocalDateNode, rv reflect.Value, path string) error {
	if rv.Type() == reflect.TypeOf(LocalDate{}) {
		rv.Set(reflect.ValueOf(n.Val))
		return nil
	}
	if rv.Type() == timeType {
		t := time.Date(n.Val.Year, time.Month(n.Val.Month), n.Val.Day, 0, 0, 0, 0, time.UTC)
		rv.Set(reflect.ValueOf(t))
		return nil
	}
	return fmt.Errorf("toml: cannot decode LocalDate into %s at %q", rv.Type(), path)
}

func (dec *decoder) decodeLocalTime(n *LocalTimeNode, rv reflect.Value, path string) error {
	if rv.Type() == reflect.TypeOf(LocalTime{}) {
		rv.Set(reflect.ValueOf(n.Val))
		return nil
	}
	return fmt.Errorf("toml: cannot decode LocalTime into %s at %q", rv.Type(), path)
}

func (dec *decoder) decodeArray(arr *ArrayNode, rv reflect.Value, path string) error {
	switch rv.Kind() {
	case reflect.Slice:
		sl := reflect.MakeSlice(rv.Type(), len(arr.Elements), len(arr.Elements))
		for i, elem := range arr.Elements {
			if err := dec.decodeValue(elem, sl.Index(i), fmt.Sprintf("%s[%d]", path, i)); err != nil {
				return err
			}
		}
		rv.Set(sl)
		return nil
	case reflect.Array:
		if rv.Len() < len(arr.Elements) {
			return fmt.Errorf("toml: array has %d elements but target array has size %d at %q", len(arr.Elements), rv.Len(), path)
		}
		for i, elem := range arr.Elements {
			if err := dec.decodeValue(elem, rv.Index(i), fmt.Sprintf("%s[%d]", path, i)); err != nil {
				return err
			}
		}
		return nil
	case reflect.Interface:
		val, err := dec.decodeToInterface(arr)
		if err != nil {
			return err
		}
		rv.Set(reflect.ValueOf(val))
		return nil
	default:
		return fmt.Errorf("toml: cannot decode Array into %s at %q", rv.Type(), path)
	}
}

func (dec *decoder) decodeInlineTable(tbl *InlineTableNode, rv reflect.Value, path string) error {
	rv = dec.indirect(rv, false)
	switch rv.Kind() {
	case reflect.Struct:
		return dec.decodeChildrenIntoStruct(tbl.Children, rv, path)
	case reflect.Map:
		if rv.IsNil() {
			rv.Set(reflect.MakeMap(rv.Type()))
		}
		for _, child := range tbl.Children {
			if kv, ok := child.(*KeyValueNode); ok {
				if err := dec.decodeKVIntoMap(kv, rv, path); err != nil {
					return err
				}
			}
		}
		return nil
	case reflect.Interface:
		m := make(map[string]any)
		for _, child := range tbl.Children {
			if kv, ok := child.(*KeyValueNode); ok {
				val, err := dec.decodeToInterface(kv.Val)
				if err != nil {
					return err
				}
				setNestedMapValue(m, kv.Key.Parts, val)
			}
		}
		rv.Set(reflect.ValueOf(m))
		return nil
	default:
		return fmt.Errorf("toml: cannot decode InlineTable into %s at %q", rv.Type(), path)
	}
}

// decodeToInterface decodes a node into a native Go value for interface{} targets.
func (dec *decoder) decodeToInterface(node Node) (any, error) {
	switch n := node.(type) {
	case *StringNode:
		return n.Val, nil
	case *IntegerNode:
		return n.Val, nil
	case *FloatNode:
		return n.Val, nil
	case *BooleanNode:
		return n.Val, nil
	case *DateTimeNode:
		return n.Val, nil
	case *LocalDateTimeNode:
		return n.Val, nil
	case *LocalDateNode:
		return n.Val, nil
	case *LocalTimeNode:
		return n.Val, nil
	case *ArrayNode:
		result := make([]any, len(n.Elements))
		for i, elem := range n.Elements {
			val, err := dec.decodeToInterface(elem)
			if err != nil {
				return nil, err
			}
			result[i] = val
		}
		return result, nil
	case *InlineTableNode:
		m := make(map[string]any)
		for _, child := range n.Children {
			if kv, ok := child.(*KeyValueNode); ok {
				val, err := dec.decodeToInterface(kv.Val)
				if err != nil {
					return nil, err
				}
				setNestedMapValue(m, kv.Key.Parts, val)
			}
		}
		return m, nil
	default:
		return nil, fmt.Errorf("toml: unexpected node type %s", node.Type())
	}
}

// --- helpers ---

// indirect dereferences pointers, allocating if nil.
func (dec *decoder) indirect(rv reflect.Value, decodingNull bool) reflect.Value {
	for rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			if decodingNull {
				return rv
			}
			rv.Set(reflect.New(rv.Type().Elem()))
		}
		rv = rv.Elem()
	}
	return rv
}

// appendPath builds dotted paths for error messages.
func appendPath(prefix, part string) string {
	if prefix == "" {
		return part
	}
	return prefix + "." + part
}

// ensureSubMap ensures a sub-map exists at the given key in the map and returns it.
func ensureSubMap(m reflect.Value, key reflect.Value) reflect.Value {
	existing := m.MapIndex(key)
	if existing.IsValid() {
		elem := existing
		if elem.Kind() == reflect.Interface {
			elem = elem.Elem()
		}
		if elem.Kind() == reflect.Map {
			return elem
		}
	}
	sub := reflect.MakeMap(m.Type())
	m.SetMapIndex(key, sub)
	got := m.MapIndex(key)
	if got.Kind() == reflect.Interface {
		got = got.Elem()
	}
	return got
}

// setNestedMapValue sets a value in a nested map[string]any following a dotted key path.
func setNestedMapValue(m map[string]any, parts []string, val any) {
	current := m
	for i, part := range parts {
		if i < len(parts)-1 {
			sub, ok := current[part]
			if !ok {
				sub = make(map[string]any)
				current[part] = sub
			}
			if sm, ok := sub.(map[string]any); ok {
				current = sm
			} else {
				return
			}
		} else {
			current[part] = val
		}
	}
}

// parseTag parses a struct tag like "name,omitempty" into (name, options).
func parseTag(tag string) (string, string) {
	if idx := strings.Index(tag, ","); idx != -1 {
		return tag[:idx], tag[idx+1:]
	}
	return tag, ""
}

// collectArrayTablePaths returns the set of single-component array-table KeyPaths
// found in the document. These are used to identify sub-tables that should be
// skipped by the top-level loop (they are handled within array-table processing).
func collectArrayTablePaths(doc *Document) [][]string {
	var paths [][]string
	seen := map[string]bool{}
	for _, child := range doc.Children {
		if at, ok := child.(*ArrayTableNode); ok {
			// Only track single-component paths as "top-level" array tables.
			// Multi-component paths (e.g., ["products","tags"]) are sub-arrays.
			if len(at.KeyPath) == 1 {
				key := at.KeyPath[0]
				if !seen[key] {
					seen[key] = true
					paths = append(paths, at.KeyPath)
				}
			}
		}
	}
	return paths
}

// isSubTableOfArrayTable returns true if keyPath is a sub-table of any array-table path,
// i.e., keyPath has the array-table path as a prefix and is longer.
func isSubTableOfArrayTable(keyPath []string, arrayTablePaths [][]string) bool {
	for _, atp := range arrayTablePaths {
		if hasPrefix(keyPath, atp) && len(keyPath) > len(atp) {
			return true
		}
	}
	return false
}
