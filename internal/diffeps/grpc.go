package diffeps

import (
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/k1LoW/runn"
	"github.com/ramsesyok/runnora-diff/jsondiff"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
)

// Capturer keeps gRPC type provenance independently of evidence mode. runn
// registers descriptors before CaptureGRPCStart, for both proto files and
// server reflection. No proto parsing or dependencies are added to jsondiff.
func (r *Recorder) Capturer(base runn.Capturer) runn.Capturer {
	return &grpcCapturer{Capturer: base, recorder: r}
}

type grpcCapturer struct {
	runn.Capturer
	recorder *Recorder
	output   protoreflect.MessageDescriptor
	err      error
}

func (c *grpcCapturer) CaptureGRPCStart(name string, typ runn.GRPCType, service, method string) {
	c.Capturer.CaptureGRPCStart(name, typ, service, method)
	c.output, c.err = nil, nil
	d, err := protoregistry.GlobalFiles.FindDescriptorByName(protoreflect.FullName(strings.TrimPrefix(service, ".") + "." + method))
	if err != nil {
		c.err = fmt.Errorf("gRPC %s/%s: response descriptor: %w", service, method, err)
		return
	}
	md, ok := d.(protoreflect.MethodDescriptor)
	if !ok {
		c.err = fmt.Errorf("gRPC %s/%s: descriptor is not a method", service, method)
		return
	}
	c.output = md.Output()
}

func (c *grpcCapturer) CaptureGRPCResponseMessage(m map[string]any) {
	c.recorder.mu.Lock()
	c.recorder.remember(m, responseSchema{message: c.output, err: c.err})
	c.recorder.mu.Unlock()
	c.Capturer.CaptureGRPCResponseMessage(m)
}

// Identity distinguishes a captured response from unrelated JSON of the same
// shape. Retain each value in responseSchema so its address cannot be reused.
// Slice length is part of the identity to support slices of repeated values.
type nodeID struct {
	kind   reflect.Kind
	ptr    uintptr
	length int
}

func identity(v any) (nodeID, bool) {
	rv := reflect.ValueOf(v)
	switch v.(type) {
	case map[string]any:
		if rv.IsNil() {
			return nodeID{}, false
		}
		return nodeID{kind: reflect.Map, ptr: uintptr(rv.UnsafePointer())}, true
	case []any:
		if rv.Len() == 0 {
			return nodeID{}, false
		}
		return nodeID{kind: reflect.Slice, ptr: rv.Pointer(), length: rv.Len()}, true
	default:
		return nodeID{}, false
	}
}

type responseSchema struct {
	value   any
	message protoreflect.MessageDescriptor
	list    bool
	numeric bool
	err     error
}

func isNumeric64(fd protoreflect.FieldDescriptor) bool {
	switch fd.Kind() {
	case protoreflect.Int64Kind, protoreflect.Uint64Kind, protoreflect.Sint64Kind, protoreflect.Fixed64Kind, protoreflect.Sfixed64Kind:
		return true
	}
	return false
}

func isNumericWrapper(md protoreflect.MessageDescriptor) bool {
	return md != nil && (md.FullName() == "google.protobuf.Int64Value" || md.FullName() == "google.protobuf.UInt64Value")
}

// remember also tags nested containers so comparing a message subtree or a
// repeated field uses its own descriptor. Map fields are deliberately excluded.
func (r *Recorder) remember(v any, schema responseSchema) {
	if id, ok := identity(v); ok {
		schema.value = v
		r.schemas[id] = schema
	}
	if schema.err != nil || schema.numeric {
		return
	}
	if schema.list {
		for _, item := range v.([]any) {
			r.remember(item, responseSchema{message: schema.message})
		}
		return
	}
	m, ok := v.(map[string]any)
	if !ok || schema.message == nil {
		return
	}
	fields := schema.message.Fields()
	for i := 0; i < fields.Len(); i++ {
		fd := fields.Get(i)
		if fd.IsMap() {
			continue
		}
		child, exists := m[string(fd.Name())] // runn emits UseProtoNames
		if !exists || child == nil {
			continue
		}
		md := fd.Message()
		numeric := isNumeric64(fd) || isNumericWrapper(md)
		if fd.IsList() {
			if _, ok := child.([]any); ok && (md != nil || numeric) {
				r.remember(child, responseSchema{message: md, list: true, numeric: numeric})
			}
		} else if md != nil && !numeric {
			r.remember(child, responseSchema{message: md})
		}
	}
}

func (r *Recorder) numericStringPaths(values ...any) ([]string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	paths := map[string]struct{}{}
	var visit func(any, jsondiff.Path) error
	visit = func(v any, path jsondiff.Path) error {
		if id, ok := identity(v); ok {
			if schema, found := r.schemas[id]; found {
				if schema.err != nil {
					return schema.err
				}
				collectNumericPaths(v, schema, path.String(), paths)
				return nil
			}
		}
		switch v := v.(type) {
		case map[string]any:
			for key, child := range v {
				if err := visit(child, childPath(path, key)); err != nil {
					return err
				}
			}
		case []any:
			for i, child := range v {
				if err := visit(child, childPath(path, i)); err != nil {
					return err
				}
			}
		case []map[string]any: // runn's res.messages before JSON normalization
			for i, child := range v {
				if err := visit(child, childPath(path, i)); err != nil {
					return err
				}
			}
		}
		return nil
	}
	for _, value := range values {
		if err := visit(value, jsondiff.Path{}); err != nil {
			return nil, err
		}
	}
	out := make([]string, 0, len(paths))
	for path := range paths {
		out = append(out, path)
	}
	sort.Strings(out)
	return out, nil
}

func childPath(path jsondiff.Path, key any) jsondiff.Path {
	return append(append(jsondiff.Path(nil), path...), key)
}

func collectNumericPaths(v any, schema responseSchema, path string, paths map[string]struct{}) {
	if v == nil {
		return
	}
	if schema.list {
		// A protobuf repeated field has one element type. Use a wildcard so
		// the number of compiled comparison rules does not grow with its length.
		if path == "." {
			path = ".[]?"
		} else {
			path += "[]?"
		}
		for _, child := range v.([]any) {
			collectNumericPaths(child, responseSchema{message: schema.message, numeric: schema.numeric}, path, paths)
		}
		return
	}
	if schema.numeric {
		paths[path] = struct{}{}
		return
	}
	m, ok := v.(map[string]any)
	if !ok || schema.message == nil {
		return
	}
	fields := schema.message.Fields()
	for i := 0; i < fields.Len(); i++ {
		fd := fields.Get(i)
		if fd.IsMap() {
			continue
		}
		child, exists := m[string(fd.Name())]
		if !exists {
			continue
		}
		next := responseSchema{message: fd.Message(), numeric: isNumeric64(fd) || isNumericWrapper(fd.Message()), list: fd.IsList()}
		if next.list {
			if _, ok := child.([]any); !ok {
				continue
			}
		}
		key := (jsondiff.Path{string(fd.Name())}).String()
		if path == "." {
			collectNumericPaths(child, next, key, paths)
		} else {
			if strings.HasPrefix(key, ".[") {
				key = strings.TrimPrefix(key, ".")
			}
			collectNumericPaths(child, next, path+key, paths)
		}
	}
}
