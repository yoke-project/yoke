package trunk

import (
	"fmt"
	"net"
	"os"
)

// Ceiling is the longest path a Unix domain socket may have: 108 bytes, the terminator included.
const Ceiling = 107

// A Listener is a bound channel.
type Listener = net.Listener

// Bind binds a socket at path, giving it the form's mode explicitly. A path beyond the ceiling is refused.
func Bind(path string, form Form) (Listener, error) {
	if len(path) > Ceiling {
		return nil, fmt.Errorf("the socket path %s is %d characters, beyond the %d a socket path may have", path, len(path), Ceiling)
	}
	listener, err := net.Listen("unix", path)
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(path, form.SocketMode()); err != nil {
		listener.Close()
		return nil, err
	}
	return listener, nil
}
