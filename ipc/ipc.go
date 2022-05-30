package ipc

import (
	"github.com/akutz/memconn"
)

type Signal uint8

const addr = "rout5"

const (
	SigHUP Signal = iota
	SigUSR1
	SigUSR2
)

func NotifyAll(s Signal) error {

}

func Notify(incoming chan Signal, s Signal) error {

}

func Process(name string, sig Signal) error {
	c, err := memconn.Dial(addr, name)
	if err != nil {
		return err
	}
	if _, wErr := c.Write([]byte{byte(sig)}); wErr != nil {
		return wErr
	}
	return nil
}
