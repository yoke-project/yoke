package release

import (
	"bytes"
	"fmt"
	"strings"
)

// InstallerName is the file the script that obtains a release is handed over as. It carries no version,
// so the newest release's copy is always at the same address.
const InstallerName = "yoke-install.sh"

// The two lines of the script a release writes in; a copy taken from the repository has them empty.
const (
	releaseLine = `YOKE_RELEASE=""`
	digestsLine = `YOKE_DIGESTS=""`
)

// StampInstaller writes the release's version and the digests of its files — one "<sha256>  <file>"
// each — into the script, which then checks that release without reading the manifest. A script that
// does not hold each line exactly once is refused, rather than handed over unable to check anything.
func StampInstaller(script []byte, version string, sums []string) ([]byte, error) {
	for _, line := range []string{releaseLine, digestsLine} {
		n := 0
		for _, held := range strings.Split(string(script), "\n") {
			if held == line {
				n++
			}
		}
		if n != 1 {
			return nil, fmt.Errorf("the installer holds %q %d times, not once", line, n)
		}
	}
	for _, s := range sums {
		if strings.ContainsAny(s, `"$`+"`\\") {
			return nil, fmt.Errorf("a digest line cannot be written into the installer: %q", s)
		}
	}
	stamped := bytes.Replace(script, []byte("\n"+releaseLine+"\n"), []byte("\nYOKE_RELEASE=\""+version+"\"\n"), 1)
	stamped = bytes.Replace(stamped, []byte("\n"+digestsLine+"\n"),
		[]byte("\nYOKE_DIGESTS=\"\n"+strings.Join(sums, "\n")+"\n\"\n"), 1)
	return stamped, nil
}
