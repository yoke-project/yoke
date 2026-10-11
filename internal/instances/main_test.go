package instances

import (
	"fmt"
	"os"
	"testing"

	"github.com/yoke-project/yoke/internal/core/instance"
)

// claimRole makes this test binary a process that holds an instance's claim, at the root named.
const claimRole = "INSTANCES_TEST_HOLD_CLAIM"

func TestMain(m *testing.M) {
	if root := os.Getenv(claimRole); root != "" {
		if _, err := instance.Claim(root, 0o700); err != nil {
			fmt.Println("no claim:", err)
			os.Exit(1)
		}
		fmt.Println("held")
		select {}
	}
	os.Exit(m.Run())
}
