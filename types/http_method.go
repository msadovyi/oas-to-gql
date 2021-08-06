package types

import (
	"errors"
	"reflect"
)

var HttpMethod = newHttpMethodRegistry()

func newHttpMethodRegistry() *httpMethodRegistry {
	return &httpMethodRegistry{
		Get:    "GET",
		Post:   "POST",
		Put:    "PUT",
		Patch:  "PATCH",
		Delete: "DELETE",
	}
}

type httpMethodRegistry struct {
	Get    string
	Post   string
	Put    string
	Patch  string
	Delete string
}

func HttpMethodsList() []string {
	var keys []string

	val := reflect.ValueOf(HttpMethod).Elem()
	for i := 0; i < val.NumField(); i++ {
		keys = append(keys, val.Type().Field(i).Name)
	}

	return keys
}

func GetHttpMethod(key string) (string, error) {
	method, ok := reflect.Indirect(reflect.ValueOf(HttpMethod)).FieldByName(key).Interface().(string)
	if ok {
		return method, nil
	}
	return "", errors.New("method " + key + " not found.")
}
