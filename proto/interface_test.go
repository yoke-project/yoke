// The checks described by interface-contract.std.md, one test per case, reading the definitions
// themselves as the other contracts' checks do.
package proto_test

import (
	"sort"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/dynamicpb"
)

const iface = "yoke.interface.v1"

func ifc(t *testing.T, files *protoregistry.Files, name string) protoreflect.MessageDescriptor {
	t.Helper()
	found, err := files.FindDescriptorByName(protoreflect.FullName(iface + "." + name))
	if err != nil {
		t.Fatalf("no message %s: %v", name, err)
	}
	md, ok := found.(protoreflect.MessageDescriptor)
	if !ok {
		t.Fatalf("%s is not a message", name)
	}
	return md
}

// The eleven operations, as the union names them, in the order the contract states them.
var interfaceOperations = []string{
	"authenticate", "read", "subscribe", "confirm", "command", "query",
	"stream_start", "stream_stop", "stream_subscribe", "stream_unsubscribe", "reclaim",
}

// std: yoke:interface-contract.01
func TestOneServiceWithOneBidirectionalMethod(t *testing.T) {
	files := definitions(t)
	var methods []string
	var attach protoreflect.MethodDescriptor
	files.RangeFilesByPackage(iface, func(file protoreflect.FileDescriptor) bool {
		for i := 0; i < file.Services().Len(); i++ {
			s := file.Services().Get(i)
			for j := 0; j < s.Methods().Len(); j++ {
				m := s.Methods().Get(j)
				methods = append(methods, string(s.Name())+"."+string(m.Name()))
				attach = m
			}
		}
		return true
	})
	sort.Strings(methods)
	same(t, "the methods", methods, []string{"Interface.Attach"})
	if attach == nil {
		return
	}
	if !attach.IsStreamingClient() || !attach.IsStreamingServer() || attach.Input().Name() != "ClientFrame" || attach.Output().Name() != "CoreFrame" {
		t.Errorf("Attach is %v %v %s %s, want a stream of ClientFrame each way to CoreFrame",
			attach.IsStreamingClient(), attach.IsStreamingServer(), attach.Input().Name(), attach.Output().Name())
	}
}

// std: yoke:interface-contract.02
func TestTheUnionHoldsTheElevenAndEveryRequestStatesTheVersion(t *testing.T) {
	files := definitions(t)
	request := ifc(t, files, "Request")
	if f := request.Fields().ByName("version"); f == nil || f.Kind() != protoreflect.Uint32Kind {
		t.Error("a request does not state the contract's version as an integer")
	}
	same(t, "the operations", union(t, request, "operation"), interfaceOperations)
	same(t, "the answers", union(t, ifc(t, files, "Response"), "answer"), interfaceOperations)
	found, err := files.FindDescriptorByName(iface + ".Contract")
	if err != nil {
		t.Fatalf("no Contract: %v", err)
	}
	if v := found.(protoreflect.EnumDescriptor).Values().ByName("CONTRACT_VERSION"); v == nil || v.Number() != 1 {
		t.Error("the contract does not state 1")
	}
	for i := 0; i < request.Fields().Len(); i++ {
		if m := request.Fields().Get(i).Message(); m != nil {
			walkFields(m, func(m protoreflect.MessageDescriptor, f protoreflect.FieldDescriptor) {
				switch f.Name() {
				case "plugin", "incarnation", "client", "person", "actor":
					t.Errorf("%s names a %s", m.FullName(), f.Name())
				}
			})
		}
	}
}

// std: yoke:interface-contract.03
func TestAnAttachmentsFramesAndTheOpeningPictureFirst(t *testing.T) {
	files := definitions(t)
	client := ifc(t, files, "ClientFrame")
	if f := client.Fields().ByName("call"); f == nil || f.Kind() != protoreflect.StringKind {
		t.Error("a client's frame carries no call identity")
	}
	same(t, "a client's frame", union(t, client, "carries"), []string{"request", "cancel"})
	core := ifc(t, files, "CoreFrame")
	if f := core.Fields().ByName("call"); f == nil || f.Kind() != protoreflect.StringKind {
		t.Error("the Core's frame carries no call identity")
	}
	same(t, "the Core's frame", union(t, core, "carries"), []string{"opening", "answer", "event", "delivery", "refusal", "completion"})
	opening := ifc(t, files, "Opening")
	same(t, "the opening", fields(opening), []string{"picture", "subscription", "version"})
	if f := opening.Fields().ByName("picture"); f == nil || f.Message() == nil || f.Message().Name() != "Snapshot" {
		t.Error("the opening picture is not a snapshot")
	}
	same(t, "a completion", fields(ifc(t, files, "Completion")), []string{"by"})
	by, err := files.FindDescriptorByName(iface + ".Completion.By")
	if err != nil {
		t.Fatalf("no Completion.By: %v", err)
	}
	same(t, "who ends a call", values(by.(protoreflect.EnumDescriptor)), []string{"BY_CALLER", "BY_CORE"})
}

// std: yoke:interface-contract.04
func TestAReadASnapshotAndThePictureAreOneRecordOverThreeKinds(t *testing.T) {
	files := definitions(t)
	same(t, "a record", union(t, ifc(t, files, "Record"), "subject"), []string{"instance", "unit", "channel"})
	same(t, "a unit's groups", fields(ifc(t, files, "UnitRecord")), []string{"declared", "observed", "addressed"})
	same(t, "a unit as declared", fields(ifc(t, files, "UnitRecord.Declared")), []string{"identity", "kind"})
	same(t, "a unit as observed", fields(ifc(t, files, "UnitRecord.Observed")), []string{"state", "incarnation", "since", "condition", "streams"})
	same(t, "what a channel may address", fields(ifc(t, files, "UnitRecord.Addressed")), []string{"streams", "commands", "queries"})
	same(t, "a channel's groups", fields(ifc(t, files, "ChannelRecord")), []string{"declared", "observed"})
	same(t, "a channel as declared", fields(ifc(t, files, "ChannelRecord.Declared")), []string{"name", "projection", "address_class", "clients"})
	same(t, "a channel as observed", fields(ifc(t, files, "ChannelRecord.Observed")), []string{"attached", "client", "suspended", "grade", "by", "reason"})
	same(t, "a snapshot", fields(ifc(t, files, "Snapshot")), []string{"at", "records"})
	same(t, "a read's answer", fields(ifc(t, files, "Records")), []string{"records"})
}

// std: yoke:interface-contract.05
func TestACommandAndAQuestionCarryAnOpaquePayloadAndComeBackCorrelated(t *testing.T) {
	files := definitions(t)
	for _, name := range []string{"Command", "Question"} {
		md := ifc(t, files, name)
		same(t, "a "+strings.ToLower(name), fields(md), []string{"unit", "type", "payload"})
		if f := md.Fields().ByName("payload"); f == nil || f.Kind() != protoreflect.BytesKind {
			t.Errorf("a %s's payload is not bytes", strings.ToLower(name))
		}
	}
	same(t, "an acknowledgement", fields(ifc(t, files, "Acknowledged")), []string{"outcome", "line"})
	outcome, err := files.FindDescriptorByName(iface + ".Acknowledged.Outcome")
	if err != nil {
		t.Fatalf("no Acknowledged.Outcome: %v", err)
	}
	same(t, "the outcomes", values(outcome.(protoreflect.EnumDescriptor)), []string{"OUTCOME_ACCEPTED", "OUTCOME_DONE", "OUTCOME_FAILED"})
	response := ifc(t, files, "Response")
	if f := response.Fields().ByName("command"); f == nil || f.Message() == nil || f.Message().Name() != "Acknowledged" {
		t.Error("a command is not answered by the unit's acknowledgement")
	}
	answered := ifc(t, files, "Answered")
	same(t, "a question's answer", fields(answered), []string{"payload"})
	if f := response.Fields().ByName("query"); f == nil || f.Message() == nil || f.Message().Name() != "Answered" {
		t.Error("a question is not answered by the unit's bytes")
	}
}

// std: yoke:interface-contract.06
func TestWhereAStreamsDataArrivesAndTheFrameOnTheConnection(t *testing.T) {
	files := definitions(t)
	where := ifc(t, files, "Delivering")
	same(t, "the answer to stream.subscribe", fields(where), []string{"delivery", "socket", "connection", "path", "flowing"})
	same(t, "where the data arrives", union(t, where, "arrives"), []string{"socket", "connection", "path"})
	delivery := ifc(t, files, "StreamDelivery")
	same(t, "a stream delivery", fields(delivery), []string{"delivery", "sequence", "sent_at", "payload"})
	for name, kind := range map[string]protoreflect.Kind{"sequence": protoreflect.Uint64Kind, "sent_at": protoreflect.Uint64Kind, "payload": protoreflect.BytesKind} {
		if f := delivery.Fields().ByName(protoreflect.Name(name)); f == nil || f.Kind() != kind {
			t.Errorf("a delivery's %s is not %v", name, kind)
		}
	}
}

var interfaceCodes = []string{
	"operation.malformed", "operation.unknown", "compat.unsupported", "auth.required", "auth.invalid",
	"channel.in_use", "channel.not_attached", "channel.suspended", "channel.not_local", "subject.unknown",
	"scope.undeclared", "scope.withheld", "unit.not_running", "unit.no_session", "unit.unanswered",
	"instance.stopping",
}

// std: yoke:interface-contract.07
func TestEveryInterfaceCodeIsStatedAndNoneOther(t *testing.T) {
	files := definitions(t)
	found, err := files.FindDescriptorByName(iface + ".Code")
	if err != nil {
		t.Fatalf("no Code: %v", err)
	}
	option, err := files.FindDescriptorByName(iface + ".code")
	if err != nil {
		t.Fatalf("no `code` option: %v", err)
	}
	extension := dynamicpb.NewExtensionType(option.(protoreflect.ExtensionDescriptor))
	types := new(protoregistry.Types)
	types.RegisterExtension(extension)
	code := found.(protoreflect.EnumDescriptor)
	var stated []string
	for i := 0; i < code.Values().Len(); i++ {
		value := code.Values().Get(i)
		if value.Number() == 0 {
			continue
		}
		raw, _ := proto.Marshal(value.Options())
		options := value.Options().ProtoReflect().Type().New().Interface()
		(proto.UnmarshalOptions{Resolver: types}).Unmarshal(raw, options)
		name, _ := proto.GetExtension(options, extension).(string)
		stated = append(stated, name)
	}
	same(t, "the codes", stated, interfaceCodes)
}

// std: yoke:interface-contract.08
func TestAnInterfaceRefusalIsACodeAMessageAndATypedDetail(t *testing.T) {
	files := definitions(t)
	refusal := ifc(t, files, "Refusal")
	for _, name := range []string{"code", "message"} {
		if f := refusal.Fields().ByName(protoreflect.Name(name)); f == nil || f.Kind() != protoreflect.StringKind {
			t.Errorf("a refusal's %s is not a string", name)
		}
	}
	same(t, "the detail", union(t, refusal, "detail"), []string{"subject", "item", "suspension"})
	same(t, "a subject", fields(ifc(t, files, "Subject")), []string{"kind", "identity", "incarnation"})
	same(t, "a suspension", fields(ifc(t, files, "Suspension")), []string{"grade", "by"})
}
