package ajson

import (
	"reflect"
	"testing"
)

func TestParseJSONObjectPath(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected []Command
	}{
		{
			name: "filtered object", path: "$.store.book{?(@.price < 10)}.title",
			expected: []Command{
				{Value: "$"}, {Value: "store"}, {Value: "book"}, { Value: "?(@.price < 10)", ApplyInCurrentNode: true}, {Value: "title"}},
		},
		{
			name: "filtered object root", path: `${?(@.name == "some_name")}`,
			expected: []Command{{Value:"$"}, {Value: `?(@.name == "some_name")`,  ApplyInCurrentNode: true}},
		},
		{
			name: "filtered object with {} inside", path: `$.foo{?(@.note == "Use { or }")}`,
			expected: []Command{{Value:"$"}, {Value: "foo"},{Value: `?(@.note == "Use { or }")`,  ApplyInCurrentNode: true}},
		},
		{
			name: "filtered", path: "$.store.book[?(@.price < 10)].title",
			expected: []Command{{Value: "$"}, {Value: "store"}, {Value: "book"}, {Value: "?(@.price < 10)"}, {Value:"title"}},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := ParseJSONPath(test.path)
			if err != nil {
				t.Errorf("Error on parseJsonPath(json, %s) as %s: %s", test.path, test.name, err.Error())
			} else if !commandSliceEqual(result, test.expected) {
				t.Errorf("Error on parseJsonPath(%s) as %s: path doesn't match\nExpected: %s\nActual: %s", test.path, test.name, commandSliceString(test.expected), commandSliceString(result))
			}
		})
	}
}

//Test suites from cburgmer/json-path-comparison
func TestJSONPathObject_suite(t *testing.T)	 {
	tests := []struct {
		name     string
		input    string
		path     string
		expected []interface{}
		wantErr  bool
	}{
		{
			name:     "Filter expression on object",
			input:    `{"foo": "bar"}`,
			path:     `${?(@.foo)}`,
			expected: []interface{}{map[string]interface{}{"foo": "bar"}}, // ["value"]
		},
		{
			name:     "Filter expression on object single result",
			input:    `{"foo": "bar", "other_key": { "value": "1", "foo": "bar" } }`,
			path:     `${?(@.foo=="bar")}`,
			expected: []interface{}{
				map[string]interface{}{"foo": "bar", "other_key": map[string]interface{}{"value": "1", "foo": "bar"}},
			},
		},
		{
			name:     "Filter expression on object multiple result",
			input:    `{"foo": "bar", "other_key": { "value": "1", "foo": "bar" } }`,
			path:     `$..{?(@.foo=="bar")}`,
			expected: []interface{}{
				map[string]interface{}{"foo": "bar", "other_key": map[string]interface{}{"value": "1", "foo": "bar"}},
				map[string]interface{}{"value": "1", "foo": "bar"},
			},
		},
		{
			name:     "Filter expression on object single result access child",
			input:    `{"foo": "bar", "other_key": { "value": "1", "foo": "bar" }, "another_key": "the_other" }`,
			path:     `${?(@.foo=="bar")}.another_key`,
			expected: []interface{}{"the_other"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			nodes, err := JSONPath([]byte(test.input), test.path)
			if (err != nil) != test.wantErr {
				t.Errorf("JSONPath() error = %v, wantErr %v. got = %v", err, test.wantErr, nodes)
				return
			}
			if test.wantErr {
				return
			}

			results := make([]interface{}, 0)
			for _, node := range nodes {
				value, err := node.Unpack()
				if err != nil {
					t.Errorf("node.Unpack(): unexpected error: %v", err)
					return
				}
				results = append(results, value)
			}
			if !reflect.DeepEqual(results, test.expected) {
				t.Errorf("JSONPath(): wrong result:\nExpected: %#+v\nActual:   %#+v", test.expected, results)
			}
		})
	}
}

