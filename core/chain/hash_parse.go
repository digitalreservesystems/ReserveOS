package chain

import "errors"

func HashFromHex(s string) (Hash, error) {
	var h Hash
	if len(s) != 64 { return h, errors.New("bad_len") }
	for i := 0; i < 32; i++ {
		a := fromHex(s[i*2])
		b := fromHex(s[i*2+1])
		if a < 0 || b < 0 { return h, errors.New("bad_hex") }
		h[i] = byte(a<<4 | b)
	}
	return h, nil
}

func fromHex(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10
	case c >= 'A' && c <= 'F':
		return int(c-'A') + 10
	default:
		return -1
	}
}
