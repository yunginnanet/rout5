package ipc

import (
	"github.com/akutz/memconn"
)

type Signal uint8

const (
	SigHUP Signal = iota
	SigUSR1
	SigUSR2
)

func NotifyAll(s Signal) error {

}

func Notify(unused interface{}, s Signal) error {

}

func Process(name string, sig Signal) error {
	memconn.Listen()
}
