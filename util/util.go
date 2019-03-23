package util

import (
	"fmt"
	"io"
	"math/rand"
	"net"
)

const RAND_CHARS = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

func RandString(length int) string {
	var a string
	for i := 0; i < length; i++ {
		idx := rand.Int31n(int32(length))
		a += RAND_CHARS[idx : idx+1]
	}
	return a
}
func PanicToError(fn func()) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("Panic: %v", r)
		}
	}()
	fn()
	return
}

func PipeThenClose(src, dst net.Conn) {
	defer dst.Close()
	io.Copy(dst, src)
	return
}
