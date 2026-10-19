package shared

// GetStr reads field out of a decoded JSON object as a string, and answers ""
// where the field is absent or holds anything other than a string.
func GetStr(m map[string]any, field string) string {
	if val, ok := m[field].(string); ok {
		return val
	}
	return ""
}
