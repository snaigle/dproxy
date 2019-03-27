package main

import (
	"encoding/binary"
	"errors"
	"fmt"
	"github.com/snaigle/dproxy/msg"
	"github.com/snaigle/dproxy/util"
	"io"
	"log"
	"net"
	"strconv"
)

var (
	errAddrType        = errors.New("socks addr type not supported")
	errVer             = errors.New("socks version not supported")
	errMethod          = errors.New("socks only support 1 method now")
	errAuthExtraData   = errors.New("socks authentication get extra data")
	errAuthMethodError = errors.New("socks authenticate method error")
	errAuthLengthError = errors.New("socks authenticate legth error")
	errReqExtraData    = errors.New("socks request get extra data")
	errCmd             = errors.New("socks command not supported")
)

var (
	controlRegistry *ControlRegistry
)

const (
	socksVer5         = 5
	socksCmdConnect   = 1
	socksAuthNone     = 0
	socksAuthUserName = 2
)

func main() {
	log.Println("server starting")
	controlRegistry = NewControlRegistry()
	go listenProxy("127.0.0.1:1091")
	listenSocks("127.0.0.1:1090")
	// todo 还需要有个query 接口(这里可以将clientId相关数据存到db或cache,然后server就可以支持分布式部署了)
}

func listenProxy(listenAddr string) {
	ln, err := net.Listen("tcp", listenAddr)
	if err != nil {
		log.Fatal(err)
	}
	log.Println("listen proxy connection")
	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Println("accept proxy:", err)
			continue
		}
		go handleTunnelConnection(conn)
	}
}

func listenSocks(listenAddr string) {
	ln, err := net.Listen("tcp", listenAddr)
	if err != nil {
		log.Fatal(err)
	}
	log.Println("listen socks5 connection")
	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Println("accept:", err)
			continue
		}
		go handleSocks5Connection(conn)
	}
}

func handleSocks5Connection(conn net.Conn) {
	closed := false
	defer func() {
		if !closed {
			conn.Close()
		}
	}()
	clientId, err := handshake(conn)
	if err != nil {
		log.Println("socks handshake:", err)
		return
	}

	rawaddr, addr, err := getRequest(conn)
	if err != nil {
		log.Println("error getting request:", err)
		return
	}
	log.Println("accept request:", addr)
	_, err = conn.Write([]byte{0x05, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x08, 0x43})
	if err != nil {
		log.Println("send connection confirmation:", err)
		return
	}
	proxy, err := getProxyConn(rawaddr, addr, clientId)
	if err != nil {
		log.Println("failed get proxy connection:", err)
		return
	}
	defer func() {
		if !closed {
			proxy.Close()
		}
	}()
	go util.PipeThenClose(conn, proxy)
	util.PipeThenClose(proxy, conn)
	closed = true
	log.Println("closed connection to", addr)
}

func handshake(conn net.Conn) (clientId string, err error) {
	const (
		idVer     = 0
		idNmethod = 1
	)
	buf := make([]byte, 258)
	var n int
	if n, err = io.ReadAtLeast(conn, buf, idNmethod+1); err != nil {
		return
	}
	if buf[idVer] != socksVer5 {
		err = errVer
		return
	}
	nmethod := int(buf[idNmethod])
	msgLen := nmethod + 2
	if n == msgLen {
		// do nothing
	} else if n < msgLen {
		if _, err = io.ReadFull(conn, buf[n:msgLen]); err != nil {
			return
		}
	} else {
		err = errAuthExtraData
		return
	}
	log.Printf("methods:%v\n", buf[idNmethod+1:msgLen])
	// no authentication required
	// 必须支持username password auth
	_, err = conn.Write([]byte{socksVer5, socksAuthUserName})
	authBuf := make([]byte, 67) // username 和password最长为32
	if n, err = io.ReadAtLeast(conn, authBuf, 2); err != nil {
		return
	}
	fmt.Println("auth version:", authBuf[0])
	userNameLength := int(authBuf[1])
	if userNameLength >= 32 {
		err = errAuthLengthError
		return
	}
	var p int
	if n < userNameLength+2 {
		if p, err = io.ReadAtLeast(conn, authBuf[n:], userNameLength+2-n); err != nil {
			return
		}
	}
	userName := string(authBuf[2 : userNameLength+2])
	log.Println("auth userName:", userName)
	var pl int
	if n+p == userNameLength+2 {
		if pl, err = io.ReadAtLeast(conn, authBuf[userNameLength+2:], 1); err != nil {
			return
		}
	}
	passwordLength := int(authBuf[userNameLength+2])
	if n+p+pl < userNameLength+3+passwordLength {
		if _, err = io.ReadFull(conn, authBuf[n+p+pl:userNameLength+3+passwordLength]); err != nil {
			return
		}
	}
	password := string(authBuf[userNameLength+3 : userNameLength+3+passwordLength])
	log.Println("auth password:", password)
	clientId = password
	_, err = conn.Write([]byte{authBuf[0], 0x00})
	return
}

func getRequest(conn net.Conn) (rawaddr []byte, host string, err error) {
	const (
		idVer   = 0
		idCmd   = 1
		idType  = 3 // address type index
		idIP0   = 4 // ip address start index
		idDmLen = 4 // domain address length index
		idDm0   = 5 // domain address start index

		typeIPv4 = 1 // type is ipv4 address
		typeDm   = 3 // type is domain address
		typeIPv6 = 4 // type is ipv6 address

		lenIPv4   = 3 + 1 + net.IPv4len + 2 // 3(ver+cmd+rsv) + 1addrType + ipv4 + 2port
		lenIPv6   = 3 + 1 + net.IPv6len + 2 // 3(ver+cmd+rsv) + 1addrType + ipv6 + 2port
		lenDmBase = 3 + 1 + 1 + 2           // 3 + 1addrType + 1addrLen + 2port, plus addrLen
	)
	buf := make([]byte, 263)
	var n int
	if n, err = io.ReadAtLeast(conn, buf, idDmLen+1); err != nil {
		return
	}
	if buf[idVer] != socksVer5 {
		err = errVer
		return
	}
	if buf[idCmd] != socksCmdConnect {
		err = errCmd
		return
	}
	reqLen := -1
	switch buf[idType] {
	case typeIPv4:
		reqLen = lenIPv4
	case typeIPv6:
		reqLen = lenIPv6
	case typeDm:
		reqLen = int(buf[idDmLen]) + lenDmBase
	default:
		err = errAddrType
		return
	}
	if n == reqLen {
		// do nothing
	} else if n < reqLen {
		if _, err = io.ReadFull(conn, buf[n:reqLen]); err != nil {
			return
		}
	}
	rawaddr = buf[idType:reqLen]
	switch buf[idType] {
	case typeIPv4:
		host = net.IP(buf[idIP0:idIP0+net.IPv4len]).String()
	case typeIPv6:
		host = net.IP(buf[idIP0:idIP0+net.IPv6len]).String()
	case typeDm:
		host = string(buf[idDm0 : idDm0+buf[idDmLen]])
	}
	port := binary.BigEndian.Uint16(buf[reqLen-2 : reqLen])
	host = net.JoinHostPort(host, strconv.Itoa(int(port)))
	return
}

func getProxyConn(rawaddr []byte, host string, clientId string) (conn net.Conn, err error) {
	ctl := controlRegistry.Get(clientId)
	if ctl == nil {
		log.Println("control is not found:", clientId)
		err = errors.New("control is not found")
		return
	}
	for i := 0; i < proxyMaxPoolSize; i++ {
		conn, err = ctl.GetProxy()
		if err != nil {
			log.Println("Failed to get proxy connection ", err)
			return
		}
		startProxyMsg := &msg.StartProxy{
			ClientAddr: host,
		}
		if err = msg.WriteMsg(conn, startProxyMsg); err != nil {
			log.Printf("Failed to write start-proxy-message: %v, attempt %d", err, i)
			conn.Close()
		} else {
			break
		}
	}
	return
}
