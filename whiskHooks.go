package corral

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
)

var whiskHooks map[os.Signal]func(*os.File)

func init() {
	whiskHooks[syscall.SIGINT] = func(out *os.File) {
		handleWhiskPause(out)
	}

	whiskHooks[syscall.SIGABRT] = func(out *os.File) {
		handleWhiskStop(out)
		//! otherwise we will never terminate if requested..
		os.Exit(1)
	}

}

func whiskActivateHooks(out *os.File) {
	capture := make(chan os.Signal, 2)
	signals := make([]os.Signal, 0)
	for sig := range whiskHooks {
		signals = append(signals, sig)
	}
	signal.Notify(capture, signals...)

	go func() {
		for {
			sig := <-capture
			if fn, ok := whiskHooks[sig]; ok {
				fn(out)
			} else {
				fmt.Printf("%+v", sig)
				os.Exit(1)
				return
			}
		}
	}()

}
