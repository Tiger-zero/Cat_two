package mcp

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

func operationQuery(operation *openAPIOperation, arguments map[string]any) url.Values {
	query := url.Values{}
	for _, parameter := range operation.Parameters {
		if parameter.In == "query" && arguments[parameter.Name] != nil {
			query.Set(parameter.Name, valueToString(arguments[parameter.Name]))
		}
	}
	return query
}

func valueToString(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case bool:
		return strconv.FormatBool(typed)
	case int:
		return strconv.Itoa(typed)
	case int8:
		return strconv.FormatInt(int64(typed), 10)
	case int16:
		return strconv.FormatInt(int64(typed), 10)
	case int32:
		return strconv.FormatInt(int64(typed), 10)
	case int64:
		return strconv.FormatInt(typed, 10)
	case uint:
		return strconv.FormatUint(uint64(typed), 10)
	case uint8:
		return strconv.FormatUint(uint64(typed), 10)
	case uint16:
		return strconv.FormatUint(uint64(typed), 10)
	case uint32:
		return strconv.FormatUint(uint64(typed), 10)
	case uint64:
		return strconv.FormatUint(typed, 10)
	case float32:
		return strconv.FormatFloat(float64(typed), 'f', -1, 32)
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	default:
		data, err := json.Marshal(value)
		if err != nil {
			return ""
		}
		return strings.Trim(string(data), `"`)
	}
}

func apiErrorMessage(statusCode int, value any) string {
	status := strings.TrimSpace(strconv.Itoa(statusCode) + " " + http.StatusText(statusCode))
	if object, ok := value.(map[string]any); ok {
		for _, key := range []string{"message", "error"} {
			message, ok := object[key].(string)
			if ok && message != "" {
				return status + ": " + message
			}
		}
	}
	return status
}
