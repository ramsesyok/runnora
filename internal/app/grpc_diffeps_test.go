package app_test

import (
	"context"
	"fmt"
	"net"
	"strings"
	"testing"

	"github.com/bufbuild/protocompile"
	"github.com/ramsesyok/runnora/internal/app"
	"github.com/ramsesyok/runnora/internal/scenario"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
	reflectionpb "google.golang.org/grpc/reflection/grpc_reflection_v1"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/dynamicpb"
)

func TestRun_GRPCNumericDiffEps(t *testing.T) {
	for _, useProto := range []bool{true, false} {
		t.Run(fmt.Sprintf("proto=%t", useProto), func(t *testing.T) {
			f := newFixture(t)
			pkg := fmt.Sprintf("runnora.numeric.integration.p%t", useProto)
			protoName := fmt.Sprintf("numeric-%t.proto", useProto)
			protoSource := fmt.Sprintf(`syntax = "proto3";
package %s;
message Request {}
message Child { int64 assetID = 1; string code = 2; }
message Response {
  int64 id = 1;
  repeated int64 ids = 2;
  repeated Child children = 3;
  Child nested = 4;
}
service NumericService {
  rpc Get(Request) returns (Response);
  rpc List(Request) returns (stream Response);
}`, pkg)
			f.write("proto/"+protoName, protoSource)
			compiler := protocompile.Compiler{Resolver: &protocompile.SourceResolver{Accessor: protocompile.SourceAccessorFromMap(map[string]string{protoName: protoSource})}}
			files, err := compiler.Compile(context.Background(), protoName)
			if err != nil {
				t.Fatal(err)
			}
			service := files[0].Services().Get(0)
			output := service.Methods().ByName("Get").Output()
			actual := `{"id":"9223372036854775807","ids":["9007199254740992","9007199254740993"],"children":[{"assetID":"123","code":"123"},{"assetID":"456","code":"456"}],"nested":{"assetID":"789","code":"789"}}`
			template := dynamicpb.NewMessage(output)
			if err := protojson.Unmarshal([]byte(actual), template); err != nil {
				t.Fatal(err)
			}
			response := func() *dynamicpb.Message { return proto.Clone(template).(*dynamicpb.Message) }
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			srv := grpc.NewServer()
			srv.RegisterService(&grpc.ServiceDesc{
				ServiceName: string(service.FullName()), HandlerType: (*interface{})(nil), Metadata: protoName,
				Methods: []grpc.MethodDesc{{MethodName: "Get", Handler: func(_ any, _ context.Context, decode func(any) error, _ grpc.UnaryServerInterceptor) (any, error) {
					req := dynamicpb.NewMessage(service.Methods().ByName("Get").Input())
					if err := decode(req); err != nil {
						return nil, err
					}
					return response(), nil
				}}},
				Streams: []grpc.StreamDesc{{StreamName: "List", ServerStreams: true, Handler: func(_ any, stream grpc.ServerStream) error {
					if err := stream.RecvMsg(dynamicpb.NewMessage(service.Methods().ByName("List").Input())); err != nil {
						return err
					}
					for range 2 {
						if err := stream.SendMsg(response()); err != nil {
							return err
						}
					}
					return nil
				}}},
			}, &struct{}{})
			localRegistry := &protoregistry.Files{}
			if err := localRegistry.RegisterFile(files[0]); err != nil {
				t.Fatal(err)
			}
			reflectionpb.RegisterServerReflectionServer(srv, reflection.NewServerV1(reflection.ServerOptions{Services: srv, DescriptorResolver: localRegistry}))
			go srv.Serve(listener) //nolint:errcheck
			t.Cleanup(srv.Stop)
			// Keep codes as strings while making all Child.assetID values numeric.
			expected := `{"id":9223372036854775807,"ids":[9007199254740992,9007199254740993],"children":[{"assetID":123,"code":"123"},{"assetID":456,"code":"456"}],"nested":{"assetID":789.005,"code":"789"}}`
			f.write("cases/expected.json", expected)
			f.write("cases/strict.json", strings.ReplaceAll(expected, "789.005", "789"))
			f.write("cases/stream.json", "["+expected+","+expected+"]")
			f.write("cases/adjacent.json", strings.ReplaceAll(strings.ReplaceAll(expected, "789.005", "789"), "9223372036854775807", "9223372036854775806"))
			f.write("cases/rules.yaml", "tolerances:\n  - path: .nested.assetID\n    abs: 0.01\n")
			protoConfig := ""
			if useProto {
				protoConfig = fmt.Sprintf("    protos:\n      - ../proto/%s\n", protoName)
			}
			runbook := fmt.Sprintf(`desc: automatic gRPC numeric comparison
runnora:
  id: GRPC-NUMERIC
runners:
  greq:
    addr: %s
    tls: false
%ssteps:
  get:
    greq:
      %s/Get:
        message: {}
    test: diffEps(loadJSON("cases/expected.json"), current.res.message, "cases/rules.yaml")
  later:
    test: diffEps(loadJSON("cases/strict.json"), steps.get.res.message)
  adjacent:
    test: '!diffEps(loadJSON("cases/adjacent.json"), steps.get.res.message)'
  nested:
    test: diffEps({"assetID":789.005,"code":"789"}, steps.get.res.message.nested, {"default":{"abs":0.01}})
  repeated:
    test: diffEps([9007199254740992,9007199254740993], steps.get.res.message.ids)
  string_field:
    test: '!diffEps({"assetID":789,"code":789}, steps.get.res.message.nested)'
  unchanged:
    test: steps.get.res.message.id == "9223372036854775807"
  stream:
    greq:
      %s/List:
        message: {}
    test: diffEps(loadJSON("cases/stream.json"), current.res.messages, {"default":{"abs":0.01}})
`, listener.Addr().String(), protoConfig, service.FullName(), service.FullName())
			path := f.write("runbooks/main.yml", runbook)
			rb, err := scenario.Read(path, f.root)
			if err != nil {
				t.Fatal(err)
			}
			report, err := app.NewRunner(testSetenv(t)).Run(context.Background(), &app.Plan{Root: f.root, Runbooks: []*scenario.Runbook{rb}})
			if err != nil {
				t.Fatalf("Run: %v; results: %+v", err, report.Results)
			}
			if report.Passed != 1 {
				t.Fatalf("report: %+v", report)
			}
		})
	}
}
