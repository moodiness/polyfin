package main

import (
	"cmp"
	"encoding/json"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"
)

// Change is one difference: where it lies, and what it is.
type Change struct {
	Where, What string
}

// OperationChanges are the changes of an operation Polyfin serves.
type OperationChanges struct {
	// Operation is the operation as the contract writes it, the current
	// one's when it has it.
	Operation string
	Changes   []Change
	// Schemas are the named schemas it reaches whose changes Schemas
	// lists.
	Schemas []string
}

// SchemaChanges are the changes of a named schema.
type SchemaChanges struct {
	Name    string
	Changes []Change
}

// ContractChanges are the changes of the operations Polyfin serves, from
// one contract to another.
type ContractChanges struct {
	// Served is how many of the operations Polyfin serves either contract
	// has.
	Served     int
	Operations []OperationChanges
	Schemas    []SchemaChanges
}

func (c ContractChanges) changed() bool { return len(c.Operations) > 0 }

// The OpenAPI document's operations' methods.
var methods = []string{"get", "put", "post", "delete", "options", "head", "patch", "trace"}

// placeholder is a path parameter, whose name differs between Polyfin's
// routes and the contract without changing the operation.
var placeholder = regexp.MustCompile(`\{[^}]*\}`)

func operationKey(method, path string) string {
	return strings.ToUpper(method) + " " + placeholder.ReplaceAllString(path, "{}")
}

// servedKeys are the keys of the operations of routes' patterns, among
// those of contracts for a pattern ending in a wildcard ("{path...}"),
// which serves every path under it. A GET route serves HEAD too, as
// http.ServeMux routes it.
func servedKeys(patterns []string, contracts ...map[string]operation) []string {
	var keys []string
	for _, pattern := range patterns {
		method, path, ok := strings.Cut(pattern, " ")
		if !ok {
			continue
		}
		served := []string{method}
		if method == "GET" {
			served = append(served, "HEAD")
		}
		for _, method := range served {
			key := operationKey(method, path)
			prefix, wildcard := strings.CutSuffix(key, "{}")
			if !wildcard || !strings.HasSuffix(path, "...}") {
				keys = append(keys, key)
				continue
			}
			for _, contract := range contracts {
				for known := range contract {
					if strings.HasPrefix(known, prefix) {
						keys = append(keys, known)
					}
				}
			}
		}
	}
	slices.Sort(keys)
	return slices.Compact(keys)
}

type operation struct {
	name string // "GET /api/v2/catalog/items/{id}"
	node map[string]any
}

// operations indexes a contract's operations by key.
func operations(doc map[string]any) map[string]operation {
	index := map[string]operation{}
	paths, _ := doc["paths"].(map[string]any)
	for path, item := range paths {
		item, _ := item.(map[string]any)
		for _, method := range methods {
			if node, ok := item[method].(map[string]any); ok {
				index[operationKey(method, path)] = operation{name: strings.ToUpper(method) + " " + path, node: node}
			}
		}
	}
	return index
}

// compareContracts lists the changes, from the contract pinned to the
// current one, of the operations of patterns: their parameters, request
// bodies and answers, with the schemas they reach through $refs.
func compareContracts(pinned, current []byte, patterns []string) (ContractChanges, error) {
	var docs [2]map[string]any
	for i, data := range [][]byte{pinned, current} {
		if err := json.Unmarshal(data, &docs[i]); err != nil {
			return ContractChanges{}, fmt.Errorf("reading a contract: %w", err)
		}
	}
	d := &differ{docs: docs, units: map[[2]string]*unit{}}
	before, after := operations(docs[0]), operations(docs[1])
	var result ContractChanges
	changedUnits := map[*unit]bool{}
	for _, key := range servedKeys(patterns, before, after) {
		a, inBefore := before[key]
		b, inAfter := after[key]
		switch {
		case !inBefore && !inAfter:
			continue
		case !inAfter:
			result.Operations = append(result.Operations, OperationChanges{Operation: a.name,
				Changes: []Change{{What: "no longer in the contract"}}})
		case !inBefore:
			result.Operations = append(result.Operations, OperationChanges{Operation: b.name,
				Changes: []Change{{What: "new in the contract"}}})
		default:
			c := &collector{}
			d.operation(c, a.node, b.node)
			reached := changedReached(c.reaches)
			if len(c.changes) == 0 && len(reached) == 0 {
				break
			}
			op := OperationChanges{Operation: b.name, Changes: c.changes}
			for _, u := range reached {
				changedUnits[u] = true
				op.Schemas = append(op.Schemas, u.name())
			}
			slices.Sort(op.Schemas)
			op.Schemas = slices.Compact(op.Schemas)
			result.Operations = append(result.Operations, op)
		}
		result.Served++
	}
	if d.err != nil {
		return ContractChanges{}, d.err
	}
	for u := range changedUnits {
		result.Schemas = append(result.Schemas, SchemaChanges{Name: u.name(), Changes: u.changes})
	}
	slices.SortFunc(result.Schemas, func(a, b SchemaChanges) int { return strings.Compare(a.Name, b.Name) })
	// By path, then method.
	path := func(op OperationChanges) string { _, path, _ := strings.Cut(op.Operation, " "); return path }
	slices.SortFunc(result.Operations, func(a, b OperationChanges) int {
		return cmp.Or(strings.Compare(path(a), path(b)), strings.Compare(a.Operation, b.Operation))
	})
	return result, nil
}

// unit is a pair of schemas named by $refs at the same place of both
// contracts, compared once however many operations reach it.
type unit struct {
	refs    [2]string
	changes []Change
	reaches []*unit
}

func (u *unit) name() string {
	before, after := refName(u.refs[0]), refName(u.refs[1])
	if before == after {
		return after
	}
	return after + " (was " + before + ")"
}

func refName(ref string) string { return ref[strings.LastIndexByte(ref, '/')+1:] }

// changedReached are the changed units reached from units.
func changedReached(units []*unit) []*unit {
	seen := map[*unit]bool{}
	var changed []*unit
	var visit func(u *unit)
	visit = func(u *unit) {
		if seen[u] {
			return
		}
		seen[u] = true
		if len(u.changes) > 0 {
			changed = append(changed, u)
		}
		for _, next := range u.reaches {
			visit(next)
		}
	}
	for _, u := range units {
		visit(u)
	}
	return changed
}

// collector gathers the changes found at one place, and the units reached
// from it.
type collector struct {
	changes []Change
	reaches []*unit
}

func (c *collector) add(where, format string, args ...any) {
	c.changes = append(c.changes, Change{Where: strings.TrimSpace(where), What: fmt.Sprintf(format, args...)})
}

type differ struct {
	docs  [2]map[string]any
	units map[[2]string]*unit
	err   error
}

// ignored are the keys that document without changing what is sent or
// answered.
var ignored = map[string]bool{"description": true, "summary": true, "title": true, "example": true, "examples": true,
	"externalDocs": true, "$comment": true, "operationId": true, "tags": true}

// operation compares an operation in both contracts.
func (d *differ) operation(c *collector, a, b map[string]any) {
	for _, key := range keys(a, b) {
		x, inA := a[key]
		y, inB := b[key]
		where := key
		if key == "requestBody" {
			where = "request body"
		}
		switch {
		case ignored[key]:
		case key == "parameters":
			d.parameters(c, d.list(0, x), d.list(1, y))
		case key == "responses":
			d.named(c, "answer", d.object(0, x), d.object(1, y))
		case !inA && d.object(1, y)["required"] == true:
			c.add(where, "added, required")
		case !inA:
			c.add(where, "added (%s)", summary(y))
		case !inB:
			c.add(where, "removed")
		default:
			d.walk(c, where, false, x, y)
		}
	}
}

// parameters compares parameters by where they are and their name.
func (d *differ) parameters(c *collector, a, b []any) {
	index := func(side int, list []any) map[string]map[string]any {
		params := map[string]map[string]any{}
		for _, p := range list {
			p := d.object(side, p)
			in, _ := p["in"].(string)
			name, _ := p["name"].(string)
			if in == "header" {
				name = strings.ToLower(name)
			}
			params[in+" "+name] = p
		}
		return params
	}
	before, after := index(0, a), index(1, b)
	for _, key := range keys(before, after) {
		x, inA := before[key]
		y, inB := after[key]
		where := "parameter " + key
		switch {
		case !inA:
			required := "optional"
			if y["required"] == true {
				required = "required"
			}
			c.add(where, "added, %s (%s)", required, summary(y["schema"]))
		case !inB:
			c.add(where, "removed")
		default:
			d.walk(c, where, false, x, y)
		}
	}
}

// named compares maps of named things: answers by status, media types,
// headers.
func (d *differ) named(c *collector, where string, a, b map[string]any) {
	for _, name := range keys(a, b) {
		x, inA := a[name]
		y, inB := b[name]
		at := where + " " + name
		switch {
		case !inA && summary(y) == "object":
			c.add(at, "added")
		case !inA:
			c.add(at, "added (%s)", summary(y))
		case !inB:
			c.add(at, "removed")
		default:
			d.walk(c, at, false, x, y)
		}
	}
}

// walk compares a and b, found at where, schemas when schema holds.
func (d *differ) walk(c *collector, where string, schema bool, a, b any) {
	refA, isRefA := ref(a)
	refB, isRefB := ref(b)
	if isRefA && isRefB {
		c.reaches = append(c.reaches, d.unit(refA, refB))
		return
	}
	if isRefA {
		a, schema = d.resolve(0, refA), true
	}
	if isRefB {
		b, schema = d.resolve(1, refB), true
	}
	switch x := a.(type) {
	case map[string]any:
		y, ok := b.(map[string]any)
		if !ok {
			c.add(where, "now %s, was %s", summary(b), summary(a))
			return
		}
		d.fields(c, where, schema, x, y)
	case []any:
		y, ok := b.([]any)
		if !ok {
			c.add(where, "now %s, was %s", summary(b), summary(a))
			return
		}
		d.items(c, where, schema, x, y)
	default:
		if !equal(a, b) {
			c.add(where, "now %s, was %s", summary(b), summary(a))
		}
	}
}

// fields compares the fields of two objects.
func (d *differ) fields(c *collector, where string, schema bool, a, b map[string]any) {
	for _, key := range keys(a, b) {
		if ignored[key] {
			continue
		}
		x, inA := a[key]
		y, inB := b[key]
		switch {
		case schema && key == "properties":
			d.properties(c, where, a, b)
		case schema && key == "required":
			d.required(c, where, a, b)
		case !schema && (key == "content" || key == "headers"):
			d.named(c, where, d.object(0, x), d.object(1, y))
		case !inA:
			c.add(where, "%s added: %s", key, summary(y))
		case !inB:
			c.add(where, "%s removed, was %s", key, summary(x))
		case key == "schema":
			d.walk(c, where+" ", true, x, y)
		case schema && key == "items":
			d.walk(c, where+"[]", true, x, y)
		case schema && key == "additionalProperties":
			d.walk(c, where+"{}", true, x, y)
		case schema && (key == "allOf" || key == "anyOf" || key == "oneOf" || key == "prefixItems" || key == "not"):
			d.walk(c, where+" "+key, true, x, y)
		default:
			d.walk(c, where+" "+key, false, x, y)
		}
	}
}

// properties compares a schema's properties, telling which added ones are
// required.
func (d *differ) properties(c *collector, where string, a, b map[string]any) {
	before, after := d.object(0, a["properties"]), d.object(1, b["properties"])
	required := texts(b["required"])
	for _, name := range keys(before, after) {
		x, inA := before[name]
		y, inB := after[name]
		at := where + "." + name
		switch {
		case !inA && slices.Contains(required, name):
			c.add(at, "added, required (%s)", summary(y))
		case !inA:
			c.add(at, "added (%s)", summary(y))
		case !inB:
			c.add(at, "removed")
		default:
			d.walk(c, at, true, x, y)
		}
	}
}

// required compares the properties a schema requires, but of those added
// or removed, which properties tells.
func (d *differ) required(c *collector, where string, a, b map[string]any) {
	before, after := texts(a["required"]), texts(b["required"])
	kept := func(name string) bool {
		_, inA := d.object(0, a["properties"])[name]
		_, inB := d.object(1, b["properties"])[name]
		return inA == inB
	}
	for _, name := range after {
		if !slices.Contains(before, name) && kept(name) {
			c.add(where+"."+name, "now required")
		}
	}
	for _, name := range before {
		if !slices.Contains(after, name) && kept(name) {
			c.add(where+"."+name, "no longer required")
		}
	}
}

// items compares lists: of values, as sets; of objects, by position.
func (d *differ) items(c *collector, where string, schema bool, a, b []any) {
	if scalars(a) && scalars(b) {
		var added, removed []string
		for _, y := range b {
			if !slices.ContainsFunc(a, func(x any) bool { return equal(x, y) }) {
				added = append(added, summary(y))
			}
		}
		for _, x := range a {
			if !slices.ContainsFunc(b, func(y any) bool { return equal(x, y) }) {
				removed = append(removed, summary(x))
			}
		}
		if len(added) > 0 {
			c.add(where, "added %s", strings.Join(added, ", "))
		}
		if len(removed) > 0 {
			c.add(where, "removed %s", strings.Join(removed, ", "))
		}
		return
	}
	for i := range max(len(a), len(b)) {
		at := fmt.Sprintf("%s[%d]", where, i)
		switch {
		case i >= len(a):
			c.add(at, "added (%s)", summary(b[i]))
		case i >= len(b):
			c.add(at, "removed")
		default:
			d.walk(c, at, schema, a[i], b[i])
		}
	}
}

// unit is the comparison of the schemas two $refs name, made once.
func (d *differ) unit(refA, refB string) *unit {
	key := [2]string{refA, refB}
	if u := d.units[key]; u != nil {
		return u
	}
	u := &unit{refs: key}
	// Kept before comparing: a schema may reach itself.
	d.units[key] = u
	c := &collector{}
	d.walk(c, "", true, d.resolve(0, refA), d.resolve(1, refB))
	u.changes, u.reaches = c.changes, c.reaches
	return u
}

// resolve is what a $ref of a contract names.
func (d *differ) resolve(side int, ref string) any {
	pointer, ok := strings.CutPrefix(ref, "#/")
	if !ok {
		d.fail(fmt.Errorf("the $ref %q is not in the contract", ref))
		return nil
	}
	var node any = d.docs[side]
	for _, segment := range strings.Split(pointer, "/") {
		segment = strings.NewReplacer("~1", "/", "~0", "~").Replace(segment)
		object, _ := node.(map[string]any)
		next, ok := object[segment]
		if !ok {
			d.fail(fmt.Errorf("the $ref %q names nothing", ref))
			return nil
		}
		node = next
	}
	return node
}

func (d *differ) fail(err error) {
	if d.err == nil {
		d.err = err
	}
}

// object is a node as an object, through its $ref if it has one.
func (d *differ) object(side int, node any) map[string]any {
	if r, ok := ref(node); ok {
		node = d.resolve(side, r)
	}
	object, _ := node.(map[string]any)
	return object
}

func (d *differ) list(side int, node any) []any {
	if r, ok := ref(node); ok {
		node = d.resolve(side, r)
	}
	list, _ := node.([]any)
	return list
}

func ref(node any) (string, bool) {
	object, _ := node.(map[string]any)
	r, ok := object["$ref"].(string)
	return r, ok
}

func keys[V any](a, b map[string]V) []string {
	all := slices.Collect(maps.Keys(a))
	for key := range b {
		if _, ok := a[key]; !ok {
			all = append(all, key)
		}
	}
	slices.Sort(all)
	return all
}

func texts(node any) []string {
	var out []string
	list, _ := node.([]any)
	for _, v := range list {
		if s, ok := v.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func scalars(list []any) bool {
	for _, v := range list {
		switch v.(type) {
		case map[string]any, []any:
			return false
		}
	}
	return true
}

func equal(a, b any) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return string(x) == string(y)
}

// summary describes a value in a few words: a schema by its type or name.
func summary(node any) string {
	switch v := node.(type) {
	case map[string]any:
		if r, ok := v["$ref"].(string); ok {
			return refName(r)
		}
		if t, ok := v["type"]; ok {
			kind := strings.Join(texts([]any{t}), "")
			if list, ok := t.([]any); ok {
				kind = strings.Join(texts(list), " or ")
			}
			if format, ok := v["format"].(string); ok {
				kind += " " + format
			}
			if items, ok := v["items"]; ok {
				kind += " of " + summary(items)
			}
			return kind
		}
		for _, key := range []string{"oneOf", "anyOf", "allOf"} {
			if _, ok := v[key]; ok {
				return key
			}
		}
		if schema, ok := v["schema"]; ok {
			return summary(schema)
		}
		return "object"
	case []any:
		return "list"
	case nil:
		return "null"
	default:
		data, _ := json.Marshal(v)
		if text := string(data); len(text) <= 60 {
			return text
		}
		return string(data[:57]) + "..."
	}
}
