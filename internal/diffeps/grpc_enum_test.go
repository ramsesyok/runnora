package diffeps

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/bufbuild/protocompile"
	"github.com/ramsesyok/runnora-diff/jsondiff"
	"google.golang.org/protobuf/reflect/protoreflect"
)

func enumDescriptor(t *testing.T) protoreflect.MessageDescriptor {
	t.Helper()
	compiler := protocompile.Compiler{Resolver: protocompile.WithStandardImports(&protocompile.SourceResolver{Accessor: protocompile.SourceAccessorFromMap(map[string]string{
		"enum.proto": `syntax = "proto3";
package runnora.enum.unit;
import "google/protobuf/struct.proto";
enum Status {
  option allow_alias = true;
  STATUS_UNSPECIFIED = 0;
  STATUS_ACTIVE = 1;
  STATUS_ENABLED = 1;
  STATUS_SUSPENDED = 2;
}
message Member {
  Status status = 1;
  int64 id = 2;
}
message Response {
  Status status = 1;
  repeated Status statuses = 2;
  Member owner = 3;
  repeated Member members = 4;
  map<string, Status> by_name = 5;
  optional Status optional_status = 6;
  google.protobuf.NullValue nothing = 7;
  string label = 8;
}`,
	})})}
	files, err := compiler.Compile(context.Background(), "enum.proto")
	if err != nil {
		t.Fatal(err)
	}
	return files[0].Messages().ByName("Response")
}

// actualEnumResponse is a response as runn emits it: enums as numbers.
func actualEnumResponse() map[string]any {
	return map[string]any{
		"status": 1, "statuses": []any{1, 2, 0},
		"owner":   map[string]any{"status": 2, "id": "9223372036854775807"},
		"members": []any{map[string]any{"status": 1, "id": "1"}, map[string]any{"status": 0, "id": "2"}},
		"by_name": map[string]any{"a": 1}, "optional_status": nil, "nothing": nil, "label": "STATUS_ACTIVE",
	}
}

func TestGRPCEnumNames(t *testing.T) {
	r := NewRecorder(t.TempDir(), func() (string, int) { return "check", 1 })
	actual := actualEnumResponse()
	r.remember(actual, responseSchema{message: enumDescriptor(t)})
	// Names, numbers and an alias mixed in one expectation.
	expected := map[string]any{
		"status": "STATUS_ACTIVE", "statuses": []any{"STATUS_ENABLED", 2, "STATUS_UNSPECIFIED"},
		"owner":   map[string]any{"status": "STATUS_SUSPENDED", "id": json.Number("9223372036854775807")},
		"members": []any{map[string]any{"status": "STATUS_ACTIVE", "id": 1}, map[string]any{"status": 0, "id": 2}},
		"by_name": map[string]any{"a": 1}, "optional_status": nil, "nothing": nil, "label": "STATUS_ACTIVE",
	}
	check := func(name string, e, a any, want bool, rules ...any) {
		t.Helper()
		got, err := r.Func()(append([]any{e, a}, rules...)...)
		if err != nil || got != want {
			t.Fatalf("%s: equal=%v, err=%v, calls=%+v", name, got, err, r.Calls())
		}
	}
	check("whole response", expected, actual, true)
	check("nested message", expected["owner"], actual["owner"], true)
	check("repeated messages", expected["members"], actual["members"], true)
	check("repeated enum", expected["statuses"], actual["statuses"], true)
	check("streaming messages", []any{expected}, []any{actual}, true)
	check("unrelated JSON stays strict", map[string]any{"status": "STATUS_ACTIVE"}, map[string]any{"status": 1}, false)

	expected["status"] = "STATUS_SUSPENDED"
	check("different value", expected, actual, false)
	d := r.Calls()[len(r.Calls())-1].Result.Differences[0]
	if d.Path != ".status" || d.Enum == nil || d.Enum.Actual != "STATUS_ACTIVE|STATUS_ENABLED" {
		t.Fatalf("difference: %+v", d)
	}
	expected["status"] = "STATUS_DELETED"
	check("unknown name", expected, actual, false)
	expected["status"] = "STATUS_ACTIVE"

	expected["by_name"] = map[string]any{"a": "STATUS_ACTIVE"}
	check("map excluded", expected, actual, false)
	expected["by_name"] = actual["by_name"]
	expected["label"] = 1
	check("string field stays strict", expected, actual, false)
	expected["label"] = "STATUS_ACTIVE"

	if actual["status"] != 1 {
		t.Fatal("response mutated")
	}
	numeric, enums, err := r.typedPaths(actual)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(numeric, []string{".members[]?.id", ".owner.id"}) {
		t.Errorf("numeric paths: %v", numeric)
	}
	var paths []string
	for _, e := range enums {
		paths = append(paths, e.Path)
	}
	if !reflect.DeepEqual(paths, []string{".members[]?.status", ".owner.status", ".status", ".statuses[]?"}) {
		t.Errorf("enum paths: %v", paths)
	}
	want := jsondiff.EnumValues{"STATUS_UNSPECIFIED": 0, "STATUS_ACTIVE": 1, "STATUS_ENABLED": 1, "STATUS_SUSPENDED": 2}
	for _, e := range enums {
		if !reflect.DeepEqual(e.Values, want) {
			t.Errorf("%s: values %v", e.Path, e.Values)
		}
	}
}

func TestGRPCEnumOptionalSet(t *testing.T) {
	r := NewRecorder(t.TempDir(), nil)
	actual := actualEnumResponse()
	actual["optional_status"] = 2
	r.remember(actual, responseSchema{message: enumDescriptor(t)})
	expected := actualEnumResponse()
	expected["optional_status"] = "STATUS_SUSPENDED"
	if got, err := r.Func()(expected, actual); err != nil || !got {
		t.Fatalf("equal=%v, err=%v, calls=%+v", got, err, r.Calls())
	}
}

func TestGRPCEnumRulesFromConfigWin(t *testing.T) {
	root := t.TempDir()
	// A rule written in the config overrides the descriptor's table at the same path.
	if err := os.WriteFile(filepath.Join(root, "rules.yaml"), []byte("enums:\n  - path: .status\n    values: {LEGACY_ACTIVE: 1}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	r := NewRecorder(root, nil)
	actual := actualEnumResponse()
	r.remember(actual, responseSchema{message: enumDescriptor(t)})
	expected := actualEnumResponse()
	expected["status"] = "LEGACY_ACTIVE"
	if got, err := r.Func()(expected, actual, "rules.yaml"); err != nil || !got {
		t.Fatalf("config rule: equal=%v, err=%v, calls=%+v", got, err, r.Calls())
	}
	expected["status"] = "STATUS_ACTIVE"
	if got, err := r.Func()(expected, actual, "rules.yaml"); err != nil || got {
		t.Fatalf("descriptor table should be overridden: equal=%v, err=%v", got, err)
	}
	// The cached config is not modified by the descriptor rules.
	opts, err := r.options("rules.yaml")
	if err != nil || len(opts.Enums) != 1 {
		t.Fatalf("cached options: %+v, %v", opts, err)
	}
}
