package manager

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/sys/unix"
	"google.golang.org/grpc/status"

	"github.com/yoke-project/yoke-sdk-go/admin"
	administrativev1 "github.com/yoke-project/yoke/proto/yoke/administrative/v1"
)

// clientSubjects are the subjects whose verbs speak to the Core, and so the only ones a shell carries.
var clientSubjects = map[string]bool{"core": true, "log": true, "plugin": true, "stream": true, "unit": true}

// opening is how long a shell waits for the Core's opening.
const opening = 10 * time.Second

// shell is `yoke shell`: one connection held on the selected instance's shell projection until the
// person leaves, each line a client verb performed on it. An interrupt ends the line in flight and
// nothing else. Nothing is persisted.
func shell(args []string, getenv func(string) string, in io.Reader, stdout, stderr io.Writer, interrupts <-chan os.Signal) int {
	c := &call{args: args, getenv: getenv, out: stdout}
	_, instance, selected, err := c.flags(nil)
	if err == nil && len(c.args) != 0 {
		err = refusef("yoke shell takes no arguments but --instance, and was given %s", strings.Join(c.args, " "))
	}
	if err != nil {
		return reported(err, stderr)
	}
	at, err := addresses(instance, selected, getenv)
	if err != nil {
		return reported(err, stderr)
	}
	if _, err := os.Stat(at.Shell); errors.Is(err, fs.ErrNotExist) {
		return reported(fmt.Errorf("the instance is not running: nothing answers at %s", at.Shell), stderr)
	}
	ctx, cancel := context.WithTimeout(context.Background(), opening)
	held, err := admin.Connect(ctx, at.Shell)
	cancel()
	if err != nil {
		return reported(connectRefusal(err), stderr)
	}
	defer held.Close()

	var printing sync.Mutex
	fmt.Fprintf(stdout, "connected as %s · connection %s\n", orNobody(held.Person), held.Connection)
	ended := make(chan struct{})
	if held.Standing != nil {
		go standing(held.Standing, stdout, &printing, ended)
	}

	var current struct {
		sync.Mutex
		cancel context.CancelFunc
	}
	go func() {
		for range interrupts {
			current.Lock()
			if current.cancel != nil {
				current.cancel()
			}
			current.Unlock()
		}
	}()

	lines := make(chan string)
	go func() {
		defer close(lines)
		scan := bufio.NewScanner(in)
		for scan.Scan() {
			lines <- scan.Text()
		}
	}()
	prompt := isTerminal(in)
	for {
		if prompt {
			printing.Lock()
			fmt.Fprint(stdout, "yoke> ")
			printing.Unlock()
		}
		var line string
		var open bool
		select {
		case line, open = <-lines:
		case <-ended:
			fmt.Fprintln(stderr, "yoke: the connection ended: the Core closed it or the link dropped")
			return Fault
		}
		if !open {
			return Done
		}
		words, err := split(line)
		if err != nil {
			reported(err, stderr)
			continue
		}
		if len(words) == 0 {
			continue
		}
		if len(words) == 1 && words[0] == "exit" {
			return Done
		}
		v, err := clientVerb(words)
		if err != nil {
			reported(err, stderr)
			continue
		}
		lineCtx, end := context.WithCancel(context.Background())
		current.Lock()
		current.cancel = end
		current.Unlock()
		printing.Lock()
		err = v.run(&call{args: words[2:], getenv: getenv, out: stdout, held: held, line: lineCtx, verb: words[0] + " " + words[1], takes: v.usage})
		printing.Unlock()
		current.Lock()
		current.cancel = nil
		current.Unlock()
		end()
		if errors.Is(err, admin.ErrEnded) {
			fmt.Fprintln(stderr, "yoke: the connection ended: the Core closed it or the link dropped")
			return Fault
		}
		if err != nil && !errors.Is(err, context.Canceled) {
			reported(err, stderr)
		}
	}
}

// clientVerb is the client verb a line's words name; anything else is refused, naming the line.
func clientVerb(words []string) (verb, error) {
	asked := strings.Join(words, " ")
	if len(words) >= 2 {
		asked = words[0] + " " + words[1]
	}
	if !clientSubjects[words[0]] {
		if verbs[words[0]] != nil || words[0] == "shell" {
			return verb{}, refusef("%q does not speak to the Core, and a shell carries only the verbs that do", asked)
		}
		return verb{}, refusef("%q is no client verb; the subjects are core, log, plugin, stream and unit", asked)
	}
	if len(words) < 2 || verbs[words[0]][words[1]].run == nil {
		return verb{}, refusef("%q is no client verb; %s has %s", asked, words[0], strings.Join(operations(words[0]), ", "))
	}
	return verbs[words[0]][words[1]], nil
}

// standing prints the standing subscription's deliveries as they arrive, each line beginning `* `,
// between one answer and the next. They are taken off the connection at once, so a verb in flight never
// waits behind them, and printed once nothing else is being printed. ended is closed when the
// connection ends.
func standing(s *admin.Subscription, out io.Writer, printing *sync.Mutex, ended chan struct{}) {
	var mu sync.Mutex
	var pending []string
	wake := make(chan struct{}, 1)
	done := make(chan struct{})
	go func() {
		defer close(ended)
		for {
			<-wake
			mu.Lock()
			batch := pending
			pending = nil
			mu.Unlock()
			if len(batch) > 0 {
				printing.Lock()
				for _, l := range batch {
					fmt.Fprintln(out, l)
				}
				printing.Unlock()
			}
			select {
			case <-done:
				mu.Lock()
				left := len(pending)
				mu.Unlock()
				if left == 0 {
					return
				}
			default:
			}
		}
	}()
	poke := func() {
		select {
		case wake <- struct{}{}:
		default:
		}
	}
	for {
		next, err := s.Next()
		if err != nil {
			close(done)
			poke()
			return
		}
		mu.Lock()
		pending = append(pending, delivered(next))
		mu.Unlock()
		poke()
	}
}

// delivered is one delivery of a subscription as one line.
func delivered(d *administrativev1.Subscribed) string {
	switch v := d.GetCarries().(type) {
	case *administrativev1.Subscribed_Snapshot:
		return fmt.Sprintf("* snapshot at %d, %d records", v.Snapshot.GetAt(), len(v.Snapshot.GetRecords()))
	case *administrativev1.Subscribed_Overflow:
		return fmt.Sprintf("* overflow: a fresh snapshot at %d, %d records", v.Overflow.GetAt(), len(v.Overflow.GetRecords()))
	case *administrativev1.Subscribed_Event:
		e := v.Event
		subject := e.GetSubject().GetKind() + ":" + e.GetSubject().GetIdentity()
		if n := e.GetSubject().GetIncarnation(); n != 0 {
			subject += fmt.Sprintf("/%d", n)
		}
		by := e.GetActor().GetClass()
		if p := e.GetActor().GetPerson(); p != "" {
			by += " " + p
		}
		line := fmt.Sprintf("* %d %s %s by %s", e.GetSeq(), e.GetType(), subject, by)
		// What the event says, where its detail is a document: a change of state names both states.
		if d := e.GetDetail(); len(d) > 0 && json.Valid(d) {
			var compact bytes.Buffer
			if json.Compact(&compact, d) == nil {
				line += " " + compact.String()
			}
		}
		return line
	}
	return "* " + d.String()
}

// split is a line's words: separated by white space, a word in single or double quotes holding it.
// Nothing else is interpreted.
func split(line string) ([]string, error) {
	var words []string
	var word strings.Builder
	inWord := false
	var quote rune
	for _, r := range line {
		switch {
		case quote != 0 && r == quote:
			quote = 0
		case quote != 0:
			word.WriteRune(r)
		case r == '\'' || r == '"':
			quote, inWord = r, true
		case r == ' ' || r == '\t':
			if inWord {
				words = append(words, word.String())
				word.Reset()
				inWord = false
			}
		default:
			word.WriteRune(r)
			inWord = true
		}
	}
	if quote != 0 {
		return nil, refusef("the line %q opens a quote it does not close", line)
	}
	if inWord {
		words = append(words, word.String())
	}
	return words, nil
}

// connectRefusal is a connection's failure to open: a refusal the Core sent, or the fault it is.
func connectRefusal(err error) error {
	for _, d := range status.Convert(err).Details() {
		if r, ok := d.(*administrativev1.Refusal); ok {
			return refusef("the connection was refused, %s: %s", r.GetCode(), r.GetMessage())
		}
	}
	return unreached(err)
}

func orNobody(person string) string {
	if person == "" {
		return "nobody established"
	}
	return person
}

// isTerminal says whether the lines come from a terminal, which is where a prompt is for.
func isTerminal(in io.Reader) bool {
	f, ok := in.(*os.File)
	if !ok {
		return false
	}
	_, err := unix.IoctlGetTermios(int(f.Fd()), unix.TCGETS)
	return err == nil
}
