package base62

const base62Alphabet = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

// Converts counter to short string to make it URL-friendly
func Encode(n uint64) string {
	if n == 0 {
		return "0"
	}

	var b []byte

	for n > 0 {
		b = append([]byte{base62Alphabet[n%62]}, b...)
		n /= 62
	}

	return string(b)
}