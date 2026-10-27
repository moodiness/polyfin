package main

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

type object = map[string]any

// contract is a small contract: two operations, the first of which
// Polyfin serves, answering an Item that names a Person.
func contract() object {
	answer := func(schema any) object {
		return object{"200": object{"description": "OK", "content": object{"application/json": object{"schema": schema}}}}
	}
	return object{
		"openapi": "3.1.0",
		"paths": object{
			"/api/v2/items/{item_id}": object{"get": object{
				"operationId": "getItem",
				"parameters": []any{
					object{"in": "path", "name": "item_id", "required": true, "schema": object{"type": "string"}},
					object{"in": "query", "name": "fields", "schema": object{"type": "string"}},
				},
				"responses": answer(object{"$ref": "#/components/schemas/Item"}),
			}},
			"/api/v2/item-summaries": object{"get": object{
				"responses": answer(object{"type": "object", "required": []any{"title"},
					"properties": object{"title": object{"type": "string"}}}),
			}},
			"/api/v2/admin/people": object{"get": object{
				"responses": answer(object{"$ref": "#/components/schemas/AdminPerson"}),
			}},
		},
		"components": object{"schemas": object{
			"Item": object{"type": "object", "required": []any{"id"}, "properties": object{
				"id":     object{"type": "string"},
				"people": object{"type": "array", "items": object{"$ref": "#/components/schemas/Person"}},
			}},
			"Person": object{"type": "object", "properties": object{
				"name": object{"type": "string", "description": "The person's name."},
			}},
			"AdminPerson": object{"type": "object", "properties": object{"email": object{"type": "string"}}},
		}},
	}
}

// served are the routes of the operations the tests' Polyfin serves.
var served = []string{"GET /api/v2/items/{id}", "GET /api/v2/item-summaries"}

func at(doc object, path ...string) object {
	for _, key := range path {
		doc = doc[key].(object)
	}
	return doc
}

func compare(t *testing.T, before, after object) ContractChanges {
	t.Helper()
	a, _ := json.Marshal(before)
	b, _ := json.Marshal(after)
	changes, err := compareContracts(a, b, served)
	if err != nil {
		t.Fatal(err)
	}
	return changes
}

func operationChanges(t *testing.T, changes ContractChanges, name string) OperationChanges {
	t.Helper()
	i := slices.IndexFunc(changes.Operations, func(op OperationChanges) bool { return op.Operation == name })
	if i < 0 {
		t.Fatalf("%s not among the changed operations %+v", name, changes.Operations)
	}
	return changes.Operations[i]
}

func hasChange(changes []Change, where, what string) bool {
	return slices.ContainsFunc(changes, func(c Change) bool { return c.Where == where && strings.HasPrefix(c.What, what) })
}

func TestASchemaChangeBehindARefIsReported(t *testing.T) {
	after := contract()
	at(after, "components", "schemas", "Person", "properties", "name")["type"] = "integer"

	changes := compare(t, contract(), after)
	op := operationChanges(t, changes, "GET /api/v2/items/{item_id}")
	if !slices.Contains(op.Schemas, "Person") {
		t.Errorf("the operation's changed schemas are %v, want Person, which its answer reaches through Item", op.Schemas)
	}
	i := slices.IndexFunc(changes.Schemas, func(s SchemaChanges) bool { return s.Name == "Person" })
	if i < 0 || !hasChange(changes.Schemas[i].Changes, ".name type", "now") {
		t.Errorf("schemas changed: %+v, want Person's name's type", changes.Schemas)
	}
}

func TestARenamedAnswerFieldIsReported(t *testing.T) {
	after := contract()
	schema := at(after, "paths", "/api/v2/item-summaries", "get", "responses", "200", "content", "application/json", "schema")
	schema["required"] = []any{"name"}
	schema["properties"] = object{"name": object{"type": "string"}}

	op := operationChanges(t, compare(t, contract(), after), "GET /api/v2/item-summaries")
	for _, want := range []Change{
		{Where: "answer 200 application/json .title", What: "removed"},
		{Where: "answer 200 application/json .name", What: "added, required"},
	} {
		if !hasChange(op.Changes, want.Where, want.What) {
			t.Errorf("changes %+v, want %+v", op.Changes, want)
		}
	}
}

func TestANewRequiredParameterIsReported(t *testing.T) {
	after := contract()
	get := at(after, "paths", "/api/v2/items/{item_id}", "get")
	get["parameters"] = append(get["parameters"].([]any),
		object{"in": "query", "name": "profile", "required": true, "schema": object{"type": "string"}})

	op := operationChanges(t, compare(t, contract(), after), "GET /api/v2/items/{item_id}")
	if !hasChange(op.Changes, "parameter query profile", "added, required") {
		t.Errorf("changes %+v, want the required parameter profile", op.Changes)
	}
}

func TestAnUnchangedContractChangesNothing(t *testing.T) {
	// Documentation alone changes nothing the apps send or read.
	after := contract()
	at(after, "components", "schemas", "Person", "properties", "name")["description"] = "Their name."
	at(after, "paths", "/api/v2/items/{item_id}", "get")["operationId"] = "readItem"

	for name, doc := range map[string]object{"same": contract(), "documentation": after} {
		if changes := compare(t, contract(), doc); changes.changed() || len(changes.Schemas) > 0 {
			t.Errorf("%s: changes %+v", name, changes)
		}
	}
}

func TestAnOperationPolyfinDoesNotServeIsIgnored(t *testing.T) {
	after := contract()
	at(after, "components", "schemas", "AdminPerson", "properties")["email"] = object{"type": "integer"}
	at(after, "paths", "/api/v2/admin/people", "get")["parameters"] = []any{
		object{"in": "query", "name": "role", "required": true, "schema": object{"type": "string"}}}

	if changes := compare(t, contract(), after); changes.changed() || len(changes.Schemas) > 0 {
		t.Errorf("changes %+v", changes)
	}
}
