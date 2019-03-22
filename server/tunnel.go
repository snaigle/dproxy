package main

import (
	"github.com/snaigle/dproxy/msg"
	"log"
	"net"
)

type Message interface{}

func handleTunnelConnection(conn net.Conn) {
	defer func() {
		if r := recover(); r != nil {
			log.Println("tunnel listener failed with error:", r)
		}
	}()
	var err error
	var rawMsg msg.Message
	if rawMsg, err = msg.ReadMsg(conn); err != nil {
		log.Println("read msg error:", err)
		conn.Close()
		return
	}
	switch m := rawMsg.(type) {
	case *msg.Auth:
		newControl(conn, m)
	case *msg.RegProxy:
		newProxy(conn, m)
	default:
		conn.Close()
	}
}
