package diffeps

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/bufbuild/protocompile"
	"github.com/k1LoW/runn"
	"github.com/ramsesyok/runnora/internal/evidence"
	"google.golang.org/protobuf/reflect/protoreflect"
)

func numericDescriptor(t *testing.T) protoreflect.MessageDescriptor {
	t.Helper()
	compiler := protocompile.Compiler{Resolver: protocompile.WithStandardImports(&protocompile.SourceResolver{Accessor: protocompile.SourceAccessorFromMap(map[string]string{
		"numeric.proto": `syntax = "proto3";
package runnora.numeric.unit;
import "google/protobuf/wrappers.proto";
message Response {
  int64 assetID = 1;
  uint64 id = 2;
  sint64 asset_id = 3;
  fixed64 sequence = 4;
  sfixed64 signed_sequence = 5;
  string code = 6;
  repeated int64 ids = 7;
  repeated Response children = 8;
  Response nested = 9;
  map<string, int64> lookup = 10;
  optional int64 optional_id = 11;
  google.protobuf.Int64Value wrapped = 12;
  repeated google.protobuf.UInt64Value wrapped_ids = 13;
}`,
	})})}
	files, err := compiler.Compile(context.Background(), "numeric.proto")
	if err != nil {
		t.Fatal(err)
	}
	return files[0].Messages().ByName("Response")
}

func TestGRPCNumericProvenance(t *testing.T) {
	r := NewRecorder(t.TempDir(), func() (string, int) { return "check", 1 })
	actual := map[string]any{
		"assetID": "9223372036854775807", "id": "18446744073709551615",
		"asset_id": "-9223372036854775808", "sequence": "18446744073709551615", "signed_sequence": "-9223372036854775808",
		"code": "123", "ids": []any{"9007199254740992", "9007199254740993"},
		"children": []any{map[string]any{"assetID": "123", "code": "123"}, map[string]any{"id": "456"}},
		"nested":   map[string]any{"assetID": "789"}, "lookup": map[string]any{"x": "123"}, "optional_id": nil,
		"wrapped": "9223372036854775807", "wrapped_ids": []any{"18446744073709551615"},
	}
	r.remember(actual, responseSchema{message: numericDescriptor(t)})
	expected := map[string]any{
		"assetID": json.Number("9223372036854775807"), "id": json.Number("18446744073709551615"),
		"asset_id": json.Number("-9223372036854775808"), "sequence": json.Number("18446744073709551615"), "signed_sequence": json.Number("-9223372036854775808"),
		"code": "123", "ids": []any{json.Number("9007199254740992"), json.Number("9007199254740993")},
		"children": []any{map[string]any{"assetID": 123, "code": "123"}, map[string]any{"id": 456}},
		"nested":   map[string]any{"assetID": 789}, "lookup": map[string]any{"x": "123"}, "optional_id": nil,
		"wrapped": json.Number("9223372036854775807"), "wrapped_ids": []any{json.Number("18446744073709551615")},
	}
	check := func(name string, e, a any, want bool, rules ...any) {
		t.Helper()
		args := append([]any{e, a}, rules...)
		got, err := r.Func()(args...)
		if err != nil || got != want {
			t.Fatalf("%s: equal=%v, err=%v, calls=%+v", name, got, err, r.Calls())
		}
	}
	check("whole response", expected, actual, true)
	check("nested subtree", expected["nested"], actual["nested"], true)
	check("repeated messages", expected["children"], actual["children"], true)
	check("repeated scalars", expected["ids"], actual["ids"], true)
	check("streaming messages", []any{expected}, []any{actual}, true)
	check("response envelope", map[string]any{"message": expected}, map[string]any{"message": actual}, true)
	check("unrelated JSON stays strict", map[string]any{"assetID": 789}, map[string]any{"assetID": "789"}, false)

	expected["code"] = 123
	check("string descriptor stays strict", expected, actual, false)
	expected["code"] = "123"
	expected["lookup"] = map[string]any{"x": 123}
	check("map excluded", expected, actual, false)
	expected["lookup"] = actual["lookup"]
	expected["assetID"] = json.Number("9223372036854775806")
	check("adjacent large integers differ", expected, actual, false)
	check("epsilon on typed integer", expected, actual, true, map[string]any{"default": map[string]any{"abs": 1}})
	expected["nested"].(map[string]any)["assetID"] = json.Number("789.005")
	check("computed decimal expectation", expected["nested"], actual["nested"], true, map[string]any{"default": map[string]any{"abs": .01}})
	if actual["assetID"] != "9223372036854775807" {
		t.Fatal("response mutated")
	}
	paths, err := r.numericStringPaths(actual)
	if err != nil || !reflect.DeepEqual(paths, []string{".assetID", ".asset_id", ".children[]?.assetID", ".children[]?.id", ".id", ".ids[]?", ".nested.assetID", ".sequence", ".signed_sequence", ".wrapped", ".wrapped_ids[]?"}) {
		t.Fatalf("paths: %v, %v", paths, err)
	}
}

func TestGRPCSparseRepeatedNesting(t *testing.T) {
	r := NewRecorder(t.TempDir(), func() (string, int) { return "check", 1 })
	a := map[string]any{"children": []any{
		map[string]any{"nested": nil},
		map[string]any{"nested": map[string]any{"ids": []any{"123"}}},
	}}
	r.remember(a, responseSchema{message: numericDescriptor(t)})
	e := map[string]any{"children": []any{
		map[string]any{"nested": nil},
		map[string]any{"nested": map[string]any{"ids": []any{123}}},
	}}
	got, err := r.Func()(e, a)
	if err != nil || !got {
		t.Fatalf("equal=%t, err=%v, result=%+v", got, err, r.Calls())
	}
}

func TestLoadJSONExact(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "expected.json"), []byte(`{"id":9223372036854775807,"ids":[18446744073709551615],"small":1.005}`), 0600); err != nil {
		t.Fatal(err)
	}
	r := NewRecorder(root, func() (string, int) { return "check", 1 })
	v, err := r.LoadJSON("expected.json")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(v)
	if !strings.Contains(string(b), "9223372036854775807") || !strings.Contains(string(b), "18446744073709551615") {
		t.Fatal(string(b))
	}
	if _, err := r.LoadJSON("absent.json"); err == nil {
		t.Fatal("missing file accepted")
	}
	if err := os.WriteFile(filepath.Join(root, "bad.json"), []byte(`1 2`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := r.LoadJSON("bad.json"); err == nil {
		t.Fatal("invalid JSON accepted")
	}
}

func TestGRPCMissingDescriptor(t *testing.T) {
	r := NewRecorder(t.TempDir(), nil)
	c := r.Capturer(evidence.New("test", evidence.ModeOff, nil))
	c.CaptureGRPCStart("greq", runn.GRPCUnary, "missing.numeric.Service", "Get")
	a := map[string]any{"id": "123"}
	c.CaptureGRPCResponseMessage(a)
	if _, err := r.Func()(map[string]any{"id": 123}, a); err == nil || !strings.Contains(err.Error(), "descriptor") {
		t.Fatalf("missing descriptor: %v", err)
	}
	if len(r.Calls()) != 0 {
		t.Fatal("invalid comparison recorded")
	}
}

func TestGRPCDoesNotModifyCachedOptions(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "rules.yaml"), []byte("default:\n  abs: 1\n"), 0600); err != nil {
		t.Fatal(err)
	}
	r := NewRecorder(root, nil)
	a := map[string]any{"assetID": "123"}
	r.remember(a, responseSchema{message: numericDescriptor(t)})
	e := map[string]any{"assetID": 123}
	if got, err := r.Func()(e, a, "rules.yaml"); err != nil || !got {
		t.Fatalf("typed response: %v, %v", got, err)
	}
	if got, err := r.Func()(e, map[string]any{"assetID": "123"}, "rules.yaml"); err != nil || got {
		t.Fatalf("unrelated JSON: %v, %v", got, err)
	}
}
