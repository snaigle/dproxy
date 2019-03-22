package util

import "math/rand"

const RAND_CHARS = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

func RandString(length int) string {
	var a string
	for i := 0; i < length; i++ {
		idx := rand.Int31n(int32(length))
		a += RAND_CHARS[idx : idx+1]
	}
	return a
}
