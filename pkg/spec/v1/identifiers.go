package v1

import "encoding/base32"

var identifierEncoding = base32.NewEncoding("abcdefghijklmnopqrstuvwxyz234567").WithPadding(base32.NoPadding)

func validContentID(value, prefix string) bool {
	if len(value) != len(prefix)+52 || value[:len(prefix)] != prefix {
		return false
	}
	_, err := identifierEncoding.DecodeString(value[len(prefix):])
	return err == nil
}
