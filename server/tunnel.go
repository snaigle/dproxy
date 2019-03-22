package main

import (
	"log"
	"net"
	"github.com/snaigle/dproxy/msg"
)

type Message interface{}

func handleTunnelConnection(conn net.Conn) {
	defer func() {
		if r := recover(); r != nil {
			log.Println("tunnel listener failed with error:", r)
		}
	}()

	var rawMsg msg.Message

}
