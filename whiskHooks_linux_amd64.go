package corral

import (
	"bytes"
	"fmt"
	"os"
	"os/signal"
	"syscall"
)

func whiskActivateHooks(out *os.File, logBuffer *bytes.Buffer) {
	capture := make(chan os.Signal, 2)
	signal.Notify(capture, syscall.SIGINT, syscall.SIGABRT, syscall.SIGUSR1, syscall.SIGUSR2)
	go func() {
		for {
			sig := <-capture
			switch sig {
			case syscall.SIGINT:
				handleWhiskPause(out)
			case syscall.SIGABRT:
				handleWhiskStop(out)
				logRemote(logBuffer.String())
				//! otherwise we will never terminate if requested..
				os.Exit(1)
				return
			case syscall.SIGUSR1:
				handleWhiskHint(out)
			case syscall.SIGUSR2:
				handleWhiskFreshen(out)
			default:
				fmt.Printf("%+v", sig)
				os.Exit(1)
				return
			}
		}
	}()

}
