package utils

import (
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	pluralize "github.com/gertd/go-pluralize"
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

func ToPascalCase(s string) string {
	var g []string

	p := strings.Fields(s)

	for _, value := range p {
		g = append(g, strings.Title(value))
	}
	return Sanitize(strings.Join(g, ""))
}

func ToCamelCase(s string) string {
	pascalCase := ToPascalCase(s)

	return LowerFirst(pascalCase)
}

func LowerFirst(s string) string {
	if s == "" {
		return ""
	}
	r, n := utf8.DecodeRuneInString(s)
	return string(unicode.ToLower(r)) + s[n:]
}

func Sanitize(s string) string {
	reg := regexp.MustCompile("[^a-zA-Z0-9]+")
	return reg.ReplaceAllString(s, "")
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
		return key + "=" + CastToString(data)
	case bool:
		return key + "=" + CastToString(data)
	case int:
		return key + "=" + CastToString(data)
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

func InferResourceNameFromPath(path string) string {
	pluralizeClient := pluralize.NewClient()
	parts := strings.Split(path, "/")
	result := ""
	openBracket := regexp.MustCompile(`^{`)

	for i, part := range parts {
		if !openBracket.MatchString(part) {
			if i+1 < len(parts) && len(parts[i+1]) > 0 && (isIdParam(parts[i+1]) || isSingularParam(part, parts[i+1])) {
				result += strings.Title(pluralizeClient.Singular(part))
			} else {
				result += strings.Title(part)
			}
		}
	}
	return result
}

func isIdParam(part string) bool {
	possibleId := regexp.MustCompile(`\{.*(id|name|key)*\}`)
	return possibleId.MatchString(part)
}

func isSingularParam(part string, nextPart string) bool {
	return "{"+pluralize.NewClient().Singular(part)+"}" == nextPart
}

func GenerateOperationId(
	method string,
	path string,
) string {
	reg, err := regexp.Compile("[^a-zA-Z0-9]+")
	if err != nil {
		log.Fatal(err)
	}
	pathProcessed := reg.ReplaceAllString(path, "")

	return strings.ToLower(method) + ToPascalCase(pathProcessed)
}
