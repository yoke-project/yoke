package registry_test

import (
	"bufio"
	"database/sql"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/yoke-project/yoke/internal/core/registry"
)

// A process that dies inside a write leaves its journal behind: the child below opens the file in the
// journal mode a first start runs in, spills an unfinished transaction to disk, and waits to be killed.
func init() {
	path := os.Getenv("TEST_REGISTRY_DIE_WRITING")
	if path == "" {
		return
	}
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=journal_mode(DELETE)&_pragma=cache_size(1)")
	if err != nil {
		os.Exit(2)
	}
	db.SetMaxOpenConns(1)
	tx, err := db.Begin()
	if err != nil {
		os.Exit(3)
	}
	tx.Exec(`CREATE TABLE ballast (b BLOB)`)
	for range 200 {
		tx.Exec(`INSERT INTO ballast VALUES (randomblob(4096))`)
	}
	os.Stdout.WriteString("writing\n")
	select {}
}

// std: yoke:the-registry.15
func TestARegistryLeftMidWriteIsRecoveredAndRead(t *testing.T) {
	path := filepath.Join(t.TempDir(), registry.File)
	r, err := registry.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	r.Close()
	// The file goes back to a rollback journal, as it is before a first start sets its own mode.
	db, _ := sql.Open("sqlite", "file:"+path)
	db.Exec(`PRAGMA journal_mode=DELETE`)
	db.Close()

	child := exec.Command(os.Args[0], "-test.run=^$")
	child.Env = append(os.Environ(), "TEST_REGISTRY_DIE_WRITING="+path)
	out, _ := child.StdoutPipe()
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	line, _ := bufio.NewReader(out).ReadString('\n')
	if strings.TrimSpace(line) != "writing" {
		t.Fatalf("the child said %q", line)
	}
	child.Process.Signal(syscall.SIGKILL)
	child.Wait()
	if _, err := os.Stat(path + "-journal"); err != nil {
		t.Skipf("the child left no journal to recover (%v)", err)
	}
	r, err = registry.Open(path)
	if err != nil {
		t.Fatalf("a Registry left mid-write is not opened: %v", err)
	}
	r.Close()
}
