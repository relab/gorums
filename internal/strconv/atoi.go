package strconv

import (
	"reflect"
	"strconv"
)

type Integer interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64 | ~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64 | ~uintptr
}

func signed[T Integer]() bool {
	var x T
	x = ^x // -1 if T is signed, 0xff..ff if T is unsigned
	return x == x>>1
}

// ParseInteger parses a string s in the given base and returns
// a value of type T. T must be an integer type (signed or unsigned).
// The base argument must be between 2 and 36, or be 0.
//
// Inspired by discussion in issue https://github.com/golang/go/issues/76223.
func ParseInteger[T Integer](s string, base int) (res T, err error) {
	rv := reflect.ValueOf(&res).Elem()
	if signed[T]() {
		var i int64
		i, err = strconv.ParseInt(s, base, rv.Type().Bits())
		res = T(i)
	} else {
		var u uint64
		u, err = strconv.ParseUint(s, base, rv.Type().Bits())
		res = T(u)
	}
	return
}
