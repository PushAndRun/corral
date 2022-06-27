package corral

import (
	"os"
	"syscall"
)

func init() {
	whiskHooks[syscall.SIGUSR1] = func(out *os.File) {
		handleWhiskHint(out)
	}

	whiskHooks[syscall.SIGUSR2] = func(out *os.File) {
		handleWhiskFreshen(out)
	}
}
