package provider

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

func cloneGenericAttributes(sample map[string]interface{}) map[string]interface{} {
	if len(sample) == 0 {
		return map[string]interface{}{}
	}
	cloned := make(map[string]interface{}, len(sample))
	for key, value := range sample {
		cloned[key] = value
	}
	return cloned
}

func configStringValue(config map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		v, ok := config[key]
		if !ok || v == nil {
			continue
		}
		if s, ok := v.(string); ok {
			s = strings.TrimSpace(s)
			if s != "" {
				return s
			}
			continue
		}
		s := strings.TrimSpace(fmt.Sprint(v))
		if s != "" {
			return s
		}
	}
	return ""
}

func providerReleaseMode() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv("GIN_MODE")), "release")
}

func detectSampleValueType(raw interface{}) string {
	switch raw.(type) {
	case string:
		return "string"
	case bool:
		return "boolean"
	case int, int32, int64, float32, float64:
		return "number"
	case json.Number:
		return "number"
	case map[string]interface{}:
		return "object"
	case []interface{}, []string:
		return sampleValueTypeArray
	default:
		return sampleValueTypeUnknown
	}
}
