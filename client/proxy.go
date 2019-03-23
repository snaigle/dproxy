package main

import (
	"github.com/snaigle/dproxy/msg"
	"github.com/snaigle/dproxy/util"
	"log"
	"net"
	"sync/atomic"
	"time"
)

const (
	pingInterval   = 5 * time.Second
	maxPongLatency = 15 * time.Second
)

var (
	tunnelAddr = "127.0.0.1:1091"
	clientId   string
)

// 代理
func main() {

	defer func() {
		if r := recover(); r != nil {
			log.Println("control recovering from failure ", r)
		}
	}()

	var (
		ctlConn net.Conn
		err     error
	)
	if ctlConn, err = net.Dial("tcp", tunnelAddr); err != nil {
		panic(err)
	}
	defer ctlConn.Close()
	auth := &msg.Auth{
		User: "authToken",
	}
	if err = msg.WriteMsg(ctlConn, auth); err != nil {
		panic(err)
	}
	var authResp msg.AuthResp
	if err = msg.ReadMsgInto(ctlConn, &authResp); err != nil {
		panic(err)
	}
	if authResp.Error != "" {
		log.Println("Failed to authenticate to server:", authResp.Error)
		return
	}
	clientId = authResp.ClientId
	log.Println("authenticated with server: client id:", clientId)
	lastPong := time.Now().UnixNano()
	go heartbeat(&lastPong, ctlConn)
	for {
		var rawMsg msg.Message
		if rawMsg, err = msg.ReadMsg(ctlConn); err != nil {
			panic(err)
		}
		switch m := rawMsg.(type) {
		case *msg.ReqProxy:
			go proxy()
		case *msg.Pong:
			atomic.StoreInt64(&lastPong, time.Now().UnixNano())
		default:
			log.Printf("Ignoring unknown control message %v\n", m)
		}
	}
}

func heartbeat(lastPongAddr *int64, conn net.Conn) {
	lastPing := time.Unix(atomic.LoadInt64(lastPongAddr)-1, 0)
	ping := time.NewTicker(pingInterval)
	pongCheck := time.NewTicker(time.Second)

	defer func() {
		conn.Close()
		ping.Stop()
		pongCheck.Stop()
	}()

	for {
		select {
		case <-pongCheck.C:
			lastPong := time.Unix(0, atomic.LoadInt64(lastPongAddr))
			needPong := lastPong.Sub(lastPing) < 0
			pongLatency := time.Since(lastPing)

			if needPong && pongLatency > maxPongLatency {
				log.Printf("Last ping: %v, Last pong: %v\n", lastPing, lastPong)
				log.Printf("Connection stale, haven't gotten PongMsg in %d seconds\n", int(pongLatency.Seconds()))
				return
			}

		case <-ping.C:
			err := msg.WriteMsg(conn, &msg.Ping{})
			if err != nil {
				log.Printf("Got error %v when writing PingMsg \n", err)
				return
			}
			lastPing = time.Now()
		}
	}
}

func proxy() {
	var (
		remoteConn net.Conn
		err        error
	)
	log.Println("start proxy:", clientId)
	remoteConn, err = net.Dial("tcp", tunnelAddr)
	if err != nil {
		log.Println("Failed to connect proxy connection:", err)
		return
	}
	defer remoteConn.Close()
	err = msg.WriteMsg(remoteConn, &msg.RegProxy{ClientId: clientId})
	if err != nil {
		log.Println("Failed to write regProxy:", err)
		return
	}
	var startProxy msg.StartProxy
	if err = msg.ReadMsgInto(remoteConn, &startProxy); err != nil {
		log.Println("server failed to write startProxy:", err)
		return
	}
	log.Println("start to connect :", startProxy.ClientAddr)
	localConn, err := net.Dial("tcp", startProxy.ClientAddr)
	if err != nil {
		log.Printf("Failed to open local conn %s,%v\n", startProxy.ClientAddr, err)
		return
	}
	defer localConn.Close()
	go util.PipeThenClose(remoteConn, localConn)
	util.PipeThenClose(localConn, remoteConn)
}
