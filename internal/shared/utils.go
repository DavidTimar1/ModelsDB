// Package shared handles utils logic and functionalities.
package shared

import (
	"encoding/json"
	"os"
)

func updateField(m, u map[string]interface{}, field string, changed *bool) {
	if val, ok := u[field]; ok {
		if m[field] != val {
			m[field] = val
			*changed = true
		}
	}
}

func updateFieldNum(m, u map[string]interface{}, field string, changed *bool) {
	if val, ok := u[field]; ok {
		switch v := val.(type) {
		case float64:
			if m[field] != v {
				m[field] = int(v)
				*changed = true
			}
		case int:
			if m[field] != v {
				m[field] = v
				*changed = true
			}
		}
	}
}

func hasVal(m map[string]interface{}, field string) bool {
	if val, ok := m[field]; ok {
		switch v := val.(type) {
		case string:
			return v != ""
		case bool:
			return v
		}
	}
	return false
}

func hasValNum(m map[string]interface{}, field string) bool {
	if val, ok := m[field]; ok {
		switch v := val.(type) {
		case float64:
			return v != 0
		case int:
			return v != 0
		}
	}
	return false
}

func GetStr(m map[string]interface{}, field string) string {
	if val, ok := m[field].(string); ok {
		return val
	}
	return ""
}

func GetBool(m map[string]interface{}, field string) bool {
	if val, ok := m[field]; ok {
		switch v := val.(type) {
		case bool:
			return v
		case float64:
			return v != 0
		case int:
			return v != 0
		case int64:
			return v != 0
		case string:
			return v == "true" || v == "1"
		}
	}
	return false
}

func favToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func GetNum(m map[string]interface{}, field string) int {
	if val, ok := m[field]; ok {
		switch v := val.(type) {
		case float64:
			return int(v)
		case int:
			return v
		case int64:
			return int(v)
		}
	}
	return 0
}

func getMapData(file string) ([]map[string]interface{}, map[string]map[string]interface{}) {
	var arr []map[string]interface{}
	b, err := os.ReadFile(file)
	if err == nil {
		json.Unmarshal(b, &arr)
	}

	m := make(map[string]map[string]interface{})
	for _, item := range arr {
		if name, ok := item["name"].(string); ok {
			m[name] = item
		}
	}
	return arr, m
}
