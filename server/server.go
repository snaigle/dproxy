package main

import (
	"encoding/binary"
	"errors"
	"io"
	"log"
	"net"
	"strconv"
)

var (
	errAddrType      = errors.New("socks addr type not supported")
	errVer           = errors.New("socks version not supported")
	errMethod        = errors.New("socks only support 1 method now")
	errAuthExtraData = errors.New("socks authentication get extra data")
	errReqExtraData  = errors.New("socks request get extra data")
	errCmd           = errors.New("socks command not supported")
)

var (
	controlRegistry *ControlRegistry
)

const (
	socksVer5       = 5
	socksCmdConnect = 1
)

func main() {
	controlRegistry = NewControlRegistry()
	go listenProxy("127.0.0.1:9090")
	listenSocks("127.0.0.1:1090")
}

func listenProxy(listenAddr string) {
	ln, err := net.Listen("tcp", listenAddr)
	if err != nil {
		log.Fatal(err)
	}
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
	go pipeThenClose(conn, proxy)
	pipeThenClose(proxy, conn)
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
	log.Println("methods:", buf[idNmethod+1])
	// no authentication required
	// todo 这里需要改成user password auth
	_, err = conn.Write([]byte{socksVer5, 0})
	if err == nil {
		clientId = "abcd"
	}
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
	conn, err = net.Dial("tcp", host)
	return
}

func pipeThenClose(src, dst net.Conn) {
	defer dst.Close()
	io.Copy(dst, src)
	return
}
