package converters

func SafeString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func SafeBool(b *bool) bool {
	if b == nil {
		return false
	}
	return *b
}
