// Package pam is `yoke-auth`: it asks the host's authentication service whether a secret is an account's,
// by opening the service's library at run time. It is the one package of the runtime that opens a
// library, so the one program built from it is dynamically linked, and nothing else may import it.
package pam

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"runtime"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
)

// Service is the name the runtime presents itself under to the host's authentication service.
const Service = "yoke"

// StacksVariable names a directory of stacks read in place of the host's own: a test's stack, needing no
// privilege. It widens nothing, since the program asks with the privileges of whoever runs it.
const StacksVariable = "YOKE_AUTH_STACKS"

// The exit statuses: the secret is the account's, it is not, or the service could not be asked.
const (
	Accepted = 0
	Refused  = 1
	Unasked  = 2
)

// The most an input may hold: an account name, a zero byte and a secret.
const limit = 64 << 10

const (
	pamSuccess       = 0
	pamConvErr       = 19
	pamPromptEchoOff = 1
)

type pamConv struct {
	conv    uintptr
	appdata uintptr
}

type pamMessage struct {
	style int32
	_     int32
	msg   *byte
}

type pamResponse struct {
	resp    uintptr
	retcode int32
	_       int32
}

// library is the host's authentication library and the two allocations its conversation needs, opened
// once.
type library struct {
	startConfdir func(service, user string, conv *pamConv, confdir string, handle *uintptr) int32
	start        func(service, user string, conv *pamConv, handle *uintptr) int32
	authenticate func(handle uintptr, flags int32) int32
	acctMgmt     func(handle uintptr, flags int32) int32
	end          func(handle uintptr, status int32) int32
	calloc       func(n, size uintptr) uintptr
	strdup       func(s string) uintptr
	conversation uintptr
}

var (
	opened  *library
	openErr error
	once    sync.Once
	// The secret the conversation answers with. One program asks once, so there is one.
	answer string
)

func open() (*library, error) {
	once.Do(func() {
		pam, err := purego.Dlopen("libpam.so.0", purego.RTLD_NOW|purego.RTLD_LOCAL)
		if err != nil {
			openErr = fmt.Errorf("the authentication library cannot be opened: %w", err)
			return
		}
		libc, err := purego.Dlopen("libc.so.6", purego.RTLD_NOW|purego.RTLD_GLOBAL)
		if err != nil {
			openErr = fmt.Errorf("the C library cannot be opened: %w", err)
			return
		}
		l := &library{}
		if _, err := purego.Dlsym(pam, "pam_start_confdir"); err == nil {
			purego.RegisterLibFunc(&l.startConfdir, pam, "pam_start_confdir")
		}
		purego.RegisterLibFunc(&l.start, pam, "pam_start")
		purego.RegisterLibFunc(&l.authenticate, pam, "pam_authenticate")
		purego.RegisterLibFunc(&l.acctMgmt, pam, "pam_acct_mgmt")
		purego.RegisterLibFunc(&l.end, pam, "pam_end")
		purego.RegisterLibFunc(&l.calloc, libc, "calloc")
		purego.RegisterLibFunc(&l.strdup, libc, "strdup")
		l.conversation = purego.NewCallback(l.converse)
		opened = l
	})
	return opened, openErr
}

// at is the memory the service handed over at an address: C memory, which the Go collector neither
// moves nor frees.
func at(p uintptr) unsafe.Pointer { return *(*unsafe.Pointer)(unsafe.Pointer(&p)) }

// converse answers every prompt that hides its echo with the secret, and refuses any prompt that echoes;
// a message only to display is answered with nothing. The answers are allocated by the C library, since
// the service frees them.
func (l *library) converse(n int32, msgs, resp, _ uintptr) int32 {
	if n <= 0 {
		return pamConvErr
	}
	asked := unsafe.Slice((**pamMessage)(at(msgs)), n)
	for _, m := range asked {
		if m.style == 2 { // a prompt that echoes asks for something other than the secret
			return pamConvErr
		}
	}
	out := l.calloc(uintptr(n), unsafe.Sizeof(pamResponse{}))
	if out == 0 {
		return pamConvErr
	}
	answers := unsafe.Slice((*pamResponse)(at(out)), n)
	for i, m := range asked {
		if m.style == pamPromptEchoOff {
			answers[i].resp = l.strdup(answer)
		}
	}
	*(*uintptr)(at(resp)) = out
	return pamSuccess
}

// begin starts a transaction for the service, from the stacks named where there is a name.
func (l *library) begin(account, stacks string) (uintptr, *pamConv, error) {
	conv := &pamConv{conv: l.conversation}
	var h uintptr
	var code int32
	switch {
	case stacks == "":
		code = l.start(Service, account, conv, &h)
	case l.startConfdir == nil:
		return 0, nil, errors.New("this authentication library cannot read a directory of stacks of its own")
	default:
		code = l.startConfdir(Service, account, conv, stacks, &h)
	}
	if code != pamSuccess {
		return 0, nil, fmt.Errorf("the service %s cannot be started: %d", Service, code)
	}
	return h, conv, nil
}

// Ask asks whether secret is account's: Accepted, Refused, or Unasked where the service could not be
// started. Whatever the stack answers once started is its answer, and never read for a reason.
func Ask(account, secret, stacks string) (int, error) {
	l, err := open()
	if err != nil {
		return Unasked, err
	}
	answer = secret
	defer func() { answer = "" }()
	h, conv, err := l.begin(account, stacks)
	if err != nil {
		return Unasked, err
	}
	code := l.authenticate(h, 0)
	if code == pamSuccess {
		code = l.acctMgmt(h, 0)
	}
	l.end(h, code)
	runtime.KeepAlive(conv)
	if code != pamSuccess {
		return Refused, nil
	}
	return Accepted, nil
}

// Probe starts and ends a transaction for the service, asking nothing.
func Probe(stacks string) error {
	l, err := open()
	if err != nil {
		return err
	}
	h, conv, err := l.begin("", stacks)
	if err != nil {
		return err
	}
	l.end(h, pamSuccess)
	runtime.KeepAlive(conv)
	return nil
}

// Run is `yoke-auth`: with no argument it reads an account name, a zero byte and a secret from in and
// answers by its exit status; with `--probe` it says whether the service can be asked. It writes nothing
// to out, and never the secret anywhere.
func Run(args []string, in io.Reader, out, errs io.Writer, getenv func(string) string) int {
	stacks := getenv(StacksVariable)
	switch {
	case len(args) == 1 && args[0] == "--probe":
		if err := Probe(stacks); err != nil {
			fmt.Fprintln(errs, "yoke-auth:", err)
			return Unasked
		}
		return Accepted
	case len(args) != 0:
		fmt.Fprintln(errs, "yoke-auth: no argument, or --probe")
		return Unasked
	}
	input, err := io.ReadAll(io.LimitReader(in, limit+1))
	if err != nil || len(input) > limit {
		fmt.Fprintln(errs, "yoke-auth: the input cannot be read whole")
		return Refused
	}
	account, secret, found := bytes.Cut(input, []byte{0})
	if !found || len(account) == 0 {
		fmt.Fprintln(errs, "yoke-auth: the input is not an account name, a zero byte and a secret")
		return Refused
	}
	verdict, err := Ask(string(account), string(secret), stacks)
	if err != nil {
		fmt.Fprintln(errs, "yoke-auth:", err)
	}
	return verdict
}
