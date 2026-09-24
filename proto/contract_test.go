// The checks described by plugin-contract.std.md, one test per case.
//
// They read the definitions themselves, compiled from source in the test, rather than the Go built
// from them: the definitions are authoritative on the bytes, and case 02 is what
// holds the committed Go to them.
package proto_test

import (
	"bytes"
	"context"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/bufbuild/protocompile"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/dynamicpb"
)

const pkg = "yoke.plugin.v1"

// definitions compiles every .proto under this directory except the test's own fixtures.
func definitions(t *testing.T) *protoregistry.Files {
	t.Helper()
	var sources []string
	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && d.Name() == "testdata" {
			return filepath.SkipDir
		}
		if strings.HasSuffix(path, ".proto") {
			sources = append(sources, filepath.ToSlash(path))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("reading the definitions: %v", err)
	}
	if len(sources) == 0 {
		t.Fatal("no definitions under proto/")
	}
	compiler := protocompile.Compiler{
		Resolver: protocompile.WithStandardImports(&protocompile.SourceResolver{ImportPaths: []string{"."}}),
	}
	compiled, err := compiler.Compile(context.Background(), sources...)
	if err != nil {
		t.Fatalf("the definitions do not compile: %v", err)
	}
	files := new(protoregistry.Files)
	for _, file := range compiled {
		if err := files.RegisterFile(file); err != nil {
			t.Fatalf("registering %s: %v", file.Path(), err)
		}
	}
	return files
}

func message(t *testing.T, files *protoregistry.Files, name string) protoreflect.MessageDescriptor {
	t.Helper()
	found, err := files.FindDescriptorByName(protoreflect.FullName(pkg + "." + name))
	if err != nil {
		t.Fatalf("no message %s: %v", name, err)
	}
	md, ok := found.(protoreflect.MessageDescriptor)
	if !ok {
		t.Fatalf("%s is not a message", name)
	}
	return md
}

func enum(t *testing.T, files *protoregistry.Files, name string) protoreflect.EnumDescriptor {
	t.Helper()
	found, err := files.FindDescriptorByName(protoreflect.FullName(pkg + "." + name))
	if err != nil {
		t.Fatalf("no enum %s: %v", name, err)
	}
	ed, ok := found.(protoreflect.EnumDescriptor)
	if !ok {
		t.Fatalf("%s is not an enum", name)
	}
	return ed
}

// fields names a message's fields, in order.
func fields(md protoreflect.MessageDescriptor) []string {
	var names []string
	for i := 0; i < md.Fields().Len(); i++ {
		names = append(names, string(md.Fields().Get(i).Name()))
	}
	return names
}

// values names an enum's values, leaving out the zero value every enum carries as unspecified.
func values(ed protoreflect.EnumDescriptor) []string {
	var names []string
	for i := 0; i < ed.Values().Len(); i++ {
		value := ed.Values().Get(i)
		if value.Number() != 0 {
			names = append(names, string(value.Name()))
		}
	}
	return names
}

// union names the members of a message's one oneof.
func union(t *testing.T, md protoreflect.MessageDescriptor, name string) []string {
	t.Helper()
	oneof := md.Oneofs().ByName(protoreflect.Name(name))
	if oneof == nil {
		t.Fatalf("%s has no union %q", md.Name(), name)
	}
	var names []string
	for i := 0; i < oneof.Fields().Len(); i++ {
		names = append(names, string(oneof.Fields().Get(i).Name()))
	}
	return names
}

func same(t *testing.T, what string, got, want []string) {
	t.Helper()
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("%s: got %v, want %v", what, got, want)
	}
}

// definitionsScript runs the one script that knows the tools and their versions.
func definitionsScript(t *testing.T, args ...string) (string, error) {
	t.Helper()
	script, err := filepath.Abs("../ci/definitions.sh")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(script); err != nil {
		t.Fatalf("no ci/definitions.sh: %v", err)
	}
	cmd := exec.Command("bash", append([]string{script}, args...)...)
	cmd.Dir = ".."
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// std: yoke:plugin-contract.01
func TestTheDefinitionsBuildLintAndAreFormatted(t *testing.T) {
	if out, err := definitionsScript(t, "check"); err != nil {
		t.Fatalf("the definitions do not build, lint and format clean: %v\n%s", err, out)
	}
}

// std: yoke:plugin-contract.02
func TestTheCommittedGoIsCurrent(t *testing.T) {
	out := t.TempDir()
	if said, err := definitionsScript(t, "generate", out); err != nil {
		t.Fatalf("generating failed: %v\n%s", err, said)
	}
	generated := map[string]bool{}
	err := filepath.WalkDir(out, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		relative, _ := filepath.Rel(out, path)
		generated[relative] = true
		fresh, _ := os.ReadFile(path)
		committed, err := os.ReadFile(relative)
		if err != nil {
			t.Errorf("%s is generated and not committed", relative)
			return nil
		}
		if !bytes.Equal(fresh, committed) {
			t.Errorf("%s is committed and differs from what the definitions generate", relative)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(generated) == 0 {
		t.Fatal("generating produced nothing")
	}
	_ = filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err == nil && strings.HasSuffix(path, ".pb.go") && !generated[path] {
			t.Errorf("%s is committed and nothing generates it", path)
		}
		return nil
	})
}

// std: yoke:plugin-contract.03
func TestThePathIntegerAndTheStatedIntegerAreOne(t *testing.T) {
	check := func(root string) (string, error) {
		cmd := exec.Command("go", "run", "./internal/contracts", root)
		out, err := cmd.CombinedOutput()
		return string(out), err
	}
	if out, err := check("."); err != nil {
		t.Errorf("the plugin contract is rejected: %v\n%s", err, out)
	}
	out, err := check("internal/contracts/testdata/disagreeing")
	if err == nil {
		t.Errorf("a contract under v2 stating 3 is accepted:\n%s", out)
	}
	if !strings.Contains(out, "2") || !strings.Contains(out, "3") || !strings.Contains(out, "yoke/sample/v2") {
		t.Errorf("the rejection does not name the contract and both numbers:\n%s", out)
	}
}

// std: yoke:plugin-contract.04
func TestTwoServicesAndNoThird(t *testing.T) {
	files := definitions(t)
	var services []string
	shapes := map[string]string{}
	files.RangeFilesByPackage(pkg, func(file protoreflect.FileDescriptor) bool {
		for i := 0; i < file.Services().Len(); i++ {
			service := file.Services().Get(i)
			services = append(services, string(service.Name()))
			if service.Methods().Len() != 1 {
				t.Errorf("%s has %d methods, want one", service.Name(), service.Methods().Len())
				continue
			}
			method := service.Methods().Get(0)
			switch {
			case method.IsStreamingClient() && method.IsStreamingServer():
				shapes[string(service.Name())] = "both"
			case !method.IsStreamingClient() && !method.IsStreamingServer():
				shapes[string(service.Name())] = "unary"
			default:
				shapes[string(service.Name())] = "one way"
			}
		}
		return true
	})
	sort.Strings(services)
	same(t, "services", services, []string{"Register", "Session"})
	if shapes["Register"] != "unary" {
		t.Errorf("Register is %q, want unary", shapes["Register"])
	}
	if shapes["Session"] != "both" {
		t.Errorf("Session is %q, want a stream in both directions", shapes["Session"])
	}
}

// std: yoke:plugin-contract.05
func TestTheRegistrationRequestCarriesTheClaim(t *testing.T) {
	files := definitions(t)
	request := message(t, files, "RegisterRequest")
	same(t, "the request", fields(request),
		[]string{"plugin", "unit", "token", "protocol", "artifact_version", "language", "sdk_line", "declared"})
	if kind := request.Fields().ByName("protocol").Kind(); kind != protoreflect.Uint32Kind {
		t.Errorf("the protocol version is %v, want an integer", kind)
	}
	same(t, "the declared surface", fields(message(t, files, "Surface")),
		[]string{"capabilities", "streams", "commands", "queries"})
	surface := message(t, files, "Surface")
	for i := 0; i < surface.Fields().Len(); i++ {
		if f := surface.Fields().Get(i); !f.IsList() || f.Kind() != protoreflect.StringKind {
			t.Errorf("%s is not a list of identifiers", f.Name())
		}
	}
}

// std: yoke:plugin-contract.06
func TestTheRegistrationAnswerSaysHowFarAndWhatWasWithheld(t *testing.T) {
	files := definitions(t)
	answer := message(t, files, "RegisterResponse")
	same(t, "the answer", fields(answer),
		[]string{"outcome", "stage", "code", "message", "session_id", "granted", "withheld", "heartbeat"})
	same(t, "the outcomes", values(enum(t, files, "RegisterResponse.Outcome")),
		[]string{"OUTCOME_ACCEPTED", "OUTCOME_ACCEPTED_WITH_RESTRICTIONS", "OUTCOME_REFUSED"})
	same(t, "the stages", values(enum(t, files, "Stage")),
		[]string{"STAGE_STRUCTURAL", "STAGE_IDENTITY", "STAGE_ADMINISTRATIVE_STATE", "STAGE_AUTHENTICATION",
			"STAGE_COMPATIBILITY", "STAGE_DECLARATION_CONSISTENCY", "STAGE_AUTHORISATION", "STAGE_UNIT_CONFLICT",
			"STAGE_SESSION_PREPARATION"})
	for _, name := range []string{"granted", "withheld"} {
		if f := answer.Fields().ByName(protoreflect.Name(name)); f == nil || f.Message() == nil ||
			f.Message().FullName() != pkg+".Surface" {
			t.Errorf("%s is not the four lists", name)
		}
	}
	same(t, "the heartbeat's terms", fields(message(t, files, "HeartbeatTerms")), []string{"interval", "tolerance"})
}

// std: yoke:plugin-contract.07
func TestTheEnvelopeCarriesFourFieldsAndOnePayload(t *testing.T) {
	files := definitions(t)
	envelope := message(t, files, "Envelope")
	same(t, "the envelope", fields(envelope),
		[]string{"message_id", "session_id", "sent_at_unix_nano", "correlation_id",
			"session", "control", "ack", "query", "data", "health", "event", "error"})
	same(t, "the payload", union(t, envelope, "payload"),
		[]string{"session", "control", "ack", "query", "data", "health", "event", "error"})
	if kind := envelope.Fields().ByName("sent_at_unix_nano").Kind(); kind != protoreflect.Int64Kind {
		t.Errorf("the sender's clock is %v, want an integer of nanoseconds", kind)
	}
}

// std: yoke:plugin-contract.08
func TestEachPayloadCarriesWhatItsFamilyAnswers(t *testing.T) {
	files := definitions(t)
	same(t, "a session message", union(t, message(t, files, "SessionMessage"), "kind"), []string{"open", "close", "revoked"})
	same(t, "an open", fields(message(t, files, "SessionMessage.Open")), nil)
	same(t, "a close", fields(message(t, files, "SessionMessage.Close")), nil)
	same(t, "a revocation", fields(message(t, files, "SessionMessage.Revoked")), []string{"cause", "line"})
	same(t, "a revocation's causes", values(enum(t, files, "SessionMessage.Revoked.Cause")),
		[]string{"CAUSE_LIVENESS_LOST", "CAUSE_PLUGIN_DISABLED", "CAUSE_SCOPE_EXCEEDED", "CAUSE_PROTOCOL_FAILURE"})

	same(t, "control", union(t, message(t, files, "Control"), "kind"), []string{"command", "activate", "stop"})
	same(t, "a command", fields(message(t, files, "Control.Command")), []string{"type", "payload"})
	same(t, "an activation", fields(message(t, files, "Control.Activate")), []string{"stream", "transport", "address"})
	same(t, "the transports", values(enum(t, files, "Control.Activate.Transport")),
		[]string{"TRANSPORT_ORDERED", "TRANSPORT_FRAMED", "TRANSPORT_SHARED_OBJECT"})
	same(t, "a stop", fields(message(t, files, "Control.Stop")), []string{"stream"})

	same(t, "an acknowledgement", fields(message(t, files, "Ack")), []string{"outcome", "line"})
	same(t, "its outcomes", values(enum(t, files, "Ack.Outcome")),
		[]string{"OUTCOME_ACCEPTED", "OUTCOME_DONE", "OUTCOME_FAILED"})

	same(t, "a query", union(t, message(t, files, "Query"), "kind"), []string{"question", "answer"})
	same(t, "a question", fields(message(t, files, "Query.Question")), []string{"type", "payload"})
	same(t, "an answer", fields(message(t, files, "Query.Answer")), []string{"payload"})

	same(t, "data", fields(message(t, files, "Data")), []string{"sequence", "payload"})
	same(t, "health", fields(message(t, files, "Health")), []string{"grade", "line"})
	same(t, "an event", fields(message(t, files, "Event")), []string{"occurrence", "severity", "line", "detail"})
	same(t, "an error", fields(message(t, files, "Error")), []string{"code", "message", "divergent", "withheld"})

	for _, opaque := range []string{"Control.Command", "Query.Question", "Query.Answer", "Data"} {
		if f := message(t, files, opaque).Fields().ByName("payload"); f == nil || f.Kind() != protoreflect.BytesKind {
			t.Errorf("%s's payload is not opaque bytes", opaque)
		}
	}
	if f := message(t, files, "Event").Fields().ByName("detail"); f == nil || f.Kind() != protoreflect.BytesKind {
		t.Error("an event's detail is not opaque bytes")
	}
}

// The twenty-three codes the plugin surface refuses with, in the order the contract states them.
var codes = []string{
	"admission.structural.malformed",
	"admission.identity.unknown_plugin",
	"admission.identity.unknown_unit",
	"admission.state.disabled",
	"admission.auth.invalid",
	"admission.auth.expired",
	"admission.auth.consumed",
	"admission.compat.unsupported",
	"admission.consistency.divergent",
	"admission.conflict.unit_live",
	"admission.session.unavailable",
	"session.unknown",
	"session.revoked",
	"session.message.malformed",
	"session.message.duplicate",
	"session.direction",
	"session.correlation.missing",
	"session.correlation.unknown",
	"scope.withheld",
	"scope.undeclared",
	"stream.inactive",
	"command.unknown",
	"query.unknown",
}

// codeOf reads the dotted name a value of Code states in its `code` option.
func codeOf(t *testing.T, files *protoregistry.Files, value protoreflect.EnumValueDescriptor) string {
	t.Helper()
	found, err := files.FindDescriptorByName(pkg + ".code")
	if err != nil {
		t.Fatalf("no `code` option: %v", err)
	}
	extension := dynamicpb.NewExtensionType(found.(protoreflect.ExtensionDescriptor))
	types := new(protoregistry.Types)
	if err := types.RegisterExtension(extension); err != nil {
		t.Fatal(err)
	}
	raw, err := proto.Marshal(value.Options())
	if err != nil {
		t.Fatal(err)
	}
	options := value.Options().ProtoReflect().Type().New().Interface()
	if err := (proto.UnmarshalOptions{Resolver: types}).Unmarshal(raw, options); err != nil {
		t.Fatal(err)
	}
	if !proto.HasExtension(options, extension) {
		return ""
	}
	return proto.GetExtension(options, extension).(string)
}

// std: yoke:plugin-contract.09
func TestEveryCodeIsStatedAndNoneOther(t *testing.T) {
	files := definitions(t)
	code := enum(t, files, "Code")
	var stated []string
	for i := 0; i < code.Values().Len(); i++ {
		value := code.Values().Get(i)
		if value.Number() == 0 {
			continue
		}
		name := codeOf(t, files, value)
		if name == "" {
			t.Errorf("%s states no dotted name", value.Name())
			continue
		}
		stated = append(stated, name)
	}
	same(t, "the codes", stated, codes)
	for _, name := range stated {
		for _, segment := range strings.Split(name, ".") {
			if segment == "" || strings.ToLower(segment) != segment || strings.ContainsAny(segment, " -") {
				t.Errorf("%s breaks the grammar at %q", name, segment)
			}
		}
	}
}

// std: yoke:plugin-contract.10
func TestARefusalIsACodeAMessageAndATypedDetail(t *testing.T) {
	files := definitions(t)
	refusal := message(t, files, "Error")
	for name, kind := range map[string]protoreflect.Kind{"code": protoreflect.StringKind, "message": protoreflect.StringKind} {
		if f := refusal.Fields().ByName(protoreflect.Name(name)); f == nil || f.Kind() != kind {
			t.Errorf("an error's %s is not a string", name)
		}
	}
	same(t, "the detail", union(t, refusal, "detail"), []string{"divergent", "withheld"})
	same(t, "a divergence", fields(message(t, files, "Error.Divergence")), []string{"items"})
	if f := message(t, files, "Error.Divergence").Fields().ByName("items"); f == nil || !f.IsList() {
		t.Error("a divergence does not name each item")
	}
	same(t, "a withheld item", fields(message(t, files, "Error.Withheld")), []string{"item"})
}
