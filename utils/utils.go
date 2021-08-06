package utils

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
)

func Contains(strings []string, str string) bool {
	for _, s := range strings {
		if s == str {
			return true
		}
	}
	return false
}

func GetRefName(ref string) string {
	arr := strings.Split(ref, "/")

	return arr[len(arr)-1]
}

func ToCamelCase(s string) string {
	var g []string

	p := strings.Fields(s)

	for _, value := range p {
		g = append(g, strings.Title(value))
	}
	return strings.Join(g, "")
}

func Sanitize(s string) (string, error) {
	reg, err := regexp.Compile("[^a-zA-Z0-9]+")
	if err != nil {
		return "", fmt.Errorf("cannot sanitize string: %v", s)
	}
	return reg.ReplaceAllString(s, ""), nil
}

func GetServerUrl(oas *openapi3.T) string {
	for _, s := range oas.Servers {
		if len(s.URL) > 0 {
			return s.URL
		}
	}
	panic("Server URL not found")
}

// have to find better solution to convert interface{} to query string..
func Serialize(data interface{}, key string) string {
	switch data.(type) {
	// Should we cover another cases?
	case string:
		return key + "=" + data.(string)
	case bool:
		return key + "=" + strconv.FormatBool(data.(bool))
	case int:
		return key + "=" + strconv.Itoa(data.(int))
	case []interface{}:
		arr := data.([]interface{})
		result := []string{}
		for i, v := range arr {
			newKey := "[" + strconv.Itoa(i) + "]"
			result = append(result, Serialize(v, key+newKey))
		}
		return strings.Join(result, "&")
	case map[string]interface{}:
		obj := data.(map[string]interface{})
		result := []string{}
		for k, v := range obj {
			newKey := "[" + k + "]"
			result = append(result, Serialize(v, key+newKey))
		}
		return strings.Join(result, "&")
	default:
		panic(fmt.Sprintf("Unknown type: %T\n", data))
	}
}

// converts interface{string || bool || int} to string
func CastToString(s interface{}) string {
	switch s.(type) {
	// Should we cover another cases?
	case string:
		return s.(string)
	case bool:
		return strconv.FormatBool(s.(bool))
	case int:
		return strconv.Itoa(s.(int))
	default:
		return ""
	}
}
