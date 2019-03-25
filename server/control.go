package main

import (
	"fmt"
	"github.com/snaigle/dproxy/msg"
	"github.com/snaigle/dproxy/util"
	"io"
	"log"
	"net"
	"runtime/debug"
	"time"
)

const (
	pingTimeoutInterval = 30 * time.Second
	connReapInterval    = 10 * time.Second
	controlWriteTimeout = 10 * time.Second
	proxyStaleDuration  = 60 * time.Second
	proxyMaxPoolSize    = 10
)

type Control struct {
	// auth message
	auth *msg.Auth

	// actual connection
	conn net.Conn

	// put a message in this channel to send it over
	// conn to the client
	out chan (msg.Message)

	// read from this channel to get the next message sent
	// to us over conn by the client
	in chan (msg.Message)

	// the last time we received a ping from the client - for heartbeats
	lastPing time.Time

	// all of the tunnels this control connection handles
	// proxy connections
	proxies chan net.Conn

	// identifier
	id string
}

func newControl(ctlConn net.Conn, authMsg *msg.Auth) {
	c := &Control{
		auth:     authMsg,
		conn:     ctlConn,
		out:      make(chan msg.Message),
		in:       make(chan msg.Message),
		proxies:  make(chan net.Conn, 10),
		lastPing: time.Now(),
	}
	c.id = util.RandString(16)
	log.Println("clientId:", c.id)
	if replaced := controlRegistry.Add(c.id, c); replaced != nil {
		log.Println("control is same :", c.id)
	}
	go c.writer()
	c.out <- &msg.AuthResp{
		ClientId: c.id,
	}
	c.out <- &msg.ReqProxy{}
	go c.manager()
	go c.reader()

}

func (c *Control) writer() {
	defer func() {
		if err := recover(); err != nil {
			log.Printf("Control::writer failed with error %v: %s\n", err, debug.Stack())
		}
	}()

	// write messages to the control channel
	for m := range c.out {
		if err := msg.WriteMsg(c.conn, m); err != nil {
			panic(err)
		}
	}
}

func (c *Control) reader() {
	defer func() {
		if err := recover(); err != nil {
			log.Printf("Control::reader failed with error %v: %s\n", err, debug.Stack())
		}
	}()

	// read messages from the control channel
	for {
		if msg, err := msg.ReadMsg(c.conn); err != nil {
			if err == io.EOF {
				log.Println("EOF")
				return
			} else {
				panic(err)
			}
		} else {
			// this can also panic during shutdown
			c.in <- msg
		}
	}
}
func (c *Control) RegisterProxy(conn net.Conn) {

	conn.SetDeadline(time.Now().Add(proxyStaleDuration))
	select {
	case c.proxies <- conn:
		log.Println("Registered")
	default:
		log.Println("Proxies buffer is full, discarding.")
		conn.Close()
	}
}

func (c *Control) manager() {
	// don't crash on panics
	defer func() {
		if err := recover(); err != nil {
			log.Printf("Control::manager failed with error %v: %s\n", err, debug.Stack())
		}
	}()

	// reaping timer for detecting heartbeat failure
	reap := time.NewTicker(connReapInterval)
	defer reap.Stop()

	for {
		select {
		case <-reap.C:
			if time.Since(c.lastPing) > pingTimeoutInterval {
				log.Printf("Lost heartbeat")
			}

		case mRaw, ok := <-c.in:
			// c.in closes to indicate shutdown
			if !ok {
				return
			}

			switch m := mRaw.(type) {
			case *msg.Ping:
				c.lastPing = time.Now()
				c.out <- &msg.Pong{}
			default:
				log.Println("msg type:", m)
			}
		}
	}
}

func (c *Control) GetProxy() (proxyConn net.Conn, err error) {
	var ok bool

	// get a proxy connection from the pool
	select {
	case proxyConn, ok = <-c.proxies:
		if !ok {
			err = fmt.Errorf("No proxy connections available, control is closing")
			return
		}
	default:
		// no proxy available in the pool, ask for one over the control channel
		log.Println("No proxy in pool, requesting proxy from control . . .")
		err = util.PanicToError(func() {
			c.out <- &msg.ReqProxy{}
			c.out <- &msg.ReqProxy{}
			c.out <- &msg.ReqProxy{}
			c.out <- &msg.ReqProxy{}
			c.out <- &msg.ReqProxy{}
		})
		if err != nil {
			return
		}

		select {
		case proxyConn, ok = <-c.proxies:
			if !ok {
				err = fmt.Errorf("No proxy connections available, control is closing")
				return
			}

		case <-time.After(pingTimeoutInterval):
			err = fmt.Errorf("Timeout trying to get proxy connection")
			return
		}
	}
	return
}

func newProxy(proxyConn net.Conn, regProxy *msg.RegProxy) {
	defer func() {
		if r := recover(); r != nil {
			log.Println("Failed with error:", r)
			proxyConn.Close()
		}
	}()

	ctl := controlRegistry.Get(regProxy.ClientId)
	if ctl == nil {
		panic("no client found for clientId:" + regProxy.ClientId)
	}
	ctl.RegisterProxy(proxyConn)
}
