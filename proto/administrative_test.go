// The checks described by administrative-contract.std.md, one test per case, reading the definitions
// themselves as the plugin contract's checks do.
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

const administrative = "yoke.administrative.v1"

func adm(t *testing.T, files *protoregistry.Files, name string) protoreflect.MessageDescriptor {
	t.Helper()
	found, err := files.FindDescriptorByName(protoreflect.FullName(administrative + "." + name))
	if err != nil {
		t.Fatalf("no message %s: %v", name, err)
	}
	md, ok := found.(protoreflect.MessageDescriptor)
	if !ok {
		t.Fatalf("%s is not a message", name)
	}
	return md
}

// The sixteen operations, as the union names them, in the order the contract states them.
var operations = []string{
	"plugin_enable", "plugin_disable", "plugin_grant", "plugin_withdraw",
	"unit_start", "unit_stop", "unit_restart", "unit_stream_start", "unit_stream_stop",
	"unit_retention_set", "unit_retention_clear", "unit_ask",
	"read", "log_query", "log_follow", "subscribe",
}

// std: yoke:administrative-contract.01
func TestTwoServicesOverOneUnionAndNoThird(t *testing.T) {
	files := definitions(t)
	methods := map[string]protoreflect.MethodDescriptor{}
	var services []string
	files.RangeFilesByPackage(administrative, func(file protoreflect.FileDescriptor) bool {
		for i := 0; i < file.Services().Len(); i++ {
			s := file.Services().Get(i)
			services = append(services, string(s.Name()))
			for j := 0; j < s.Methods().Len(); j++ {
				m := s.Methods().Get(j)
				methods[string(s.Name())+"."+string(m.Name())] = m
			}
		}
		return true
	})
	sort.Strings(services)
	same(t, "services", services, []string{"Operator", "Shell"})
	var names []string
	for name := range methods {
		names = append(names, name)
	}
	sort.Strings(names)
	same(t, "methods", names, []string{"Operator.Call", "Operator.Watch", "Shell.Connect"})
	shape := func(name string) (bool, bool, string, string) {
		m := methods[name]
		if m == nil {
			return false, false, "", ""
		}
		return m.IsStreamingClient(), m.IsStreamingServer(), string(m.Input().Name()), string(m.Output().Name())
	}
	if in, out, req, resp := shape("Operator.Call"); in || out || req != "Request" || resp != "Response" {
		t.Errorf("Call is %v %v %s %s", in, out, req, resp)
	}
	if in, out, req, resp := shape("Operator.Watch"); in || !out || req != "Request" || resp != "Response" {
		t.Errorf("Watch is %v %v %s %s", in, out, req, resp)
	}
	if in, out, _, _ := shape("Shell.Connect"); !in || !out {
		t.Errorf("Connect is %v %v, want a stream each way", in, out)
	}
}

// std: yoke:administrative-contract.02
func TestTheUnionHoldsTheSixteenAndEveryRequestStatesTheVersion(t *testing.T) {
	files := definitions(t)
	request := adm(t, files, "Request")
	if f := request.Fields().ByName("version"); f == nil || f.Kind() != protoreflect.Uint32Kind {
		t.Error("a request does not state the contract's version as an integer")
	}
	same(t, "the operations", union(t, request, "operation"), operations)
	same(t, "the answers", union(t, adm(t, files, "Response"), "answer"), operations)
	found, err := files.FindDescriptorByName(administrative + ".Contract")
	if err != nil {
		t.Fatalf("no Contract: %v", err)
	}
	if v := found.(protoreflect.EnumDescriptor).Values().ByName("CONTRACT_VERSION"); v == nil || v.Number() != 1 {
		t.Error("the contract does not state 1")
	}
}

// std: yoke:administrative-contract.03
func TestEveryChangeAnswersWhatItReplacedWhenAndToWhom(t *testing.T) {
	files := definitions(t)
	response := adm(t, files, "Response")
	for _, op := range operations[:11] {
		if f := response.Fields().ByName(protoreflect.Name(op)); f == nil || f.Message() == nil || f.Message().Name() != "Changed" {
			t.Errorf("the answer to %s is not a change", op)
		}
	}
	same(t, "a change", fields(adm(t, files, "Changed")), []string{"previously", "effective", "consequences"})
	if f := adm(t, files, "Changed").Fields().ByName("consequences"); f == nil || !f.IsList() {
		t.Error("the consequences are not a list")
	}
	same(t, "a consequence", fields(adm(t, files, "Consequence")), []string{"unit", "incarnation", "what"})
	effective, err := files.FindDescriptorByName(administrative + ".Changed.Effective")
	if err != nil {
		t.Fatalf("no Changed.Effective: %v", err)
	}
	same(t, "when a change takes effect", values(effective.(protoreflect.EnumDescriptor)), []string{"EFFECTIVE_IMMEDIATELY", "EFFECTIVE_AT_NEXT_ADMISSION"})
	files.RangeFilesByPackage(administrative, func(file protoreflect.FileDescriptor) bool {
		for i := 0; i < file.Messages().Len(); i++ {
			walkFields(file.Messages().Get(i), func(m protoreflect.MessageDescriptor, f protoreflect.FieldDescriptor) {
				if strings.HasPrefix(string(m.FullName()), administrative+".Request") && f.Name() == "state" {
					t.Errorf("%s carries a state", m.FullName())
				}
			})
		}
		return true
	})
}

func walkFields(m protoreflect.MessageDescriptor, visit func(protoreflect.MessageDescriptor, protoreflect.FieldDescriptor)) {
	for i := 0; i < m.Fields().Len(); i++ {
		visit(m, m.Fields().Get(i))
	}
	for i := 0; i < m.Messages().Len(); i++ {
		walkFields(m.Messages().Get(i), visit)
	}
}

// std: yoke:administrative-contract.04
func TestAReadAndASnapshotAreMadeOfOneRecord(t *testing.T) {
	files := definitions(t)
	same(t, "a record", union(t, adm(t, files, "Record"), "subject"), []string{"instance", "unit", "plugin", "channel", "document", "connection"})
	same(t, "a plugin's groups", fields(adm(t, files, "PluginRecord")), []string{"declared", "authorized", "observed"})
	same(t, "a unit's groups", fields(adm(t, files, "UnitRecord")), []string{"declared", "observed", "plugin"})
	if f := adm(t, files, "UnitRecord").Fields().ByName("plugin"); f == nil || f.Message() == nil || f.Message().Name() != "PluginRecord" {
		t.Error("a unit's record does not carry its plugin's")
	}
	same(t, "a snapshot", fields(adm(t, files, "Snapshot")), []string{"at", "records"})
	same(t, "a read's answer", fields(adm(t, files, "Records")), []string{"records"})
}

// std: yoke:administrative-contract.05
func TestTheFrameAHeldConnectionAdds(t *testing.T) {
	files := definitions(t)
	client := adm(t, files, "ClientFrame")
	if f := client.Fields().ByName("call"); f == nil || f.Kind() != protoreflect.StringKind {
		t.Error("a client's frame carries no call identity")
	}
	same(t, "a client's frame", union(t, client, "carries"), []string{"request", "cancel"})
	core := adm(t, files, "CoreFrame")
	if f := core.Fields().ByName("call"); f == nil || f.Kind() != protoreflect.StringKind {
		t.Error("the Core's frame carries no call identity")
	}
	same(t, "the Core's frame", union(t, core, "carries"), []string{"opening", "answer", "event", "refusal", "completion"})
	same(t, "the opening", fields(adm(t, files, "Opening")), []string{"connection", "actor", "subscription", "version"})
}

var administrativeCodes = []string{
	"operation.malformed", "operation.unknown", "compat.unsupported", "subject.unknown", "subject.wrong_kind",
	"capability.undeclared", "stream.undeclared", "scope.withheld", "unit.not_running", "unit.no_session",
	"unit.unanswered", "retention.invalid", "instance.stopping", "backend.unavailable", "store.unavailable",
	"scope.undeclared", "unit.failed",
}

// std: yoke:administrative-contract.06
func TestEveryAdministrativeCodeIsStatedAndNoneOther(t *testing.T) {
	files := definitions(t)
	found, err := files.FindDescriptorByName(administrative + ".Code")
	if err != nil {
		t.Fatalf("no Code: %v", err)
	}
	option, err := files.FindDescriptorByName(administrative + ".code")
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
	same(t, "the codes", stated, administrativeCodes)
}

// std: yoke:administrative-contract.07
func TestAnAdministrativeRefusalIsACodeAMessageAndATypedDetail(t *testing.T) {
	files := definitions(t)
	refusal := adm(t, files, "Refusal")
	for _, name := range []string{"code", "message"} {
		if f := refusal.Fields().ByName(protoreflect.Name(name)); f == nil || f.Kind() != protoreflect.StringKind {
			t.Errorf("a refusal's %s is not a string", name)
		}
	}
	same(t, "the detail", union(t, refusal, "detail"), []string{"subject", "item"})
	same(t, "a subject", fields(adm(t, files, "Subject")), []string{"kind", "identity", "incarnation"})
}
