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
	signal.Notify(capture, syscall.SIGINT, syscall.SIGABRT)
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
			default:
				fmt.Printf("%+v", sig)
				os.Exit(1)
				return
			}
		}
	}()

}
