package validate

import "net/url"

// Checks whether a string is a valid URL (Must be http/https and have a host).
func URL(s string) bool {
	u, err := url.ParseRequestURI(s)

	if err != nil {
		return false
	}

	if u.Scheme != "http" && u.Scheme != "https" {
		return false
	}

	if u.Host == "" {
		return false
	}

	return true
}

// Checks whether an alias is safe to use as a short code.
func Alias(s string) bool {
	if len(s) < 5 || len(s) > 20 {
		return false
	}

	for _, r := range s {
		isLetter := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
		isDigit := r >= '0' && r <= '9'
		if !isLetter && !isDigit && r != '-' && r != '_' {
			return false
		}
	}
	
	return true
}
