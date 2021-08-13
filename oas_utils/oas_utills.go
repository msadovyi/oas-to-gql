package oas_utils

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"openapi-to-graphql/types"
	"openapi-to-graphql/utils"
	"strconv"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/graphql-go/graphql"
)

type Body struct {
	ContentType string
	Data        interface{}
}

func (b *Body) Encode() io.Reader {
	switch b.ContentType {
	case "application/json":
		var jsonStr, err = json.Marshal(b.Data)
		if err != nil {
			return &bytes.Buffer{}
		}
		return bytes.NewBuffer([]byte(jsonStr))
	case "application/x-www-form-urlencoded":
		serialized := utils.Serialize(b.Data, "")
		return bytes.NewBuffer([]byte(serialized))
	}

	return &bytes.Buffer{}
}

func GetResolver(client http.Client, path string, httpMethod string, argToParam map[string]*openapi3.ParameterRef, requestBodyDef *types.RequestBodyDefinition) func(p graphql.ResolveParams) (interface{}, error) {
	return func(p graphql.ResolveParams) (interface{}, error) {
		endpoint := ExtractRequestDataFromArgs(p, path, httpMethod, argToParam)

		requestBodyValue := p.Args[requestBodyDef.ArgumentName]

		body := Body{
			ContentType: requestBodyDef.ContentType,
			Data:        requestBodyValue,
		}

		request, err := http.NewRequest(strings.ToUpper(httpMethod), endpoint, body.Encode())
		if err != nil {
			return nil, err
		}

		if requestBodyDef != nil {
			request.Header.Set("Content-Type", requestBodyDef.ContentType)
		}

		response, err := client.Do(request)
		if err != nil {
			return nil, err
		}

		defer response.Body.Close()

		responseBody, err := ioutil.ReadAll(response.Body)
		if err != nil {
			return nil, err
		}
		var jsonData interface{}

		json.Unmarshal(responseBody, &jsonData)

		text := string(responseBody) // have to check response header

		var data interface{}

		if jsonData != nil {
			data = jsonData
		} else {
			data = text
		}

		if response.StatusCode >= 400 {
			err := fmt.Errorf("StatusCode: %v. Status: %v. Response body: %v", response.StatusCode, response.Status, data)
			return nil, err
		}

		return data, nil
	}
}

func GetSuccessResponse(
	responses openapi3.Responses,
) (openapi3.ResponseRef, error) {
	for codeStr, response := range responses {
		code, err := strconv.Atoi(codeStr)
		if err == nil && code >= 200 && code < 300 && response != nil {
			return *response, nil
		}
	}
	return openapi3.ResponseRef{}, errors.New("success status code not found")
}

func GetResponseContent(
	response openapi3.ResponseRef,
) (openapi3.MediaType, error) {
	for name, content := range response.Value.Content {
		if name == "application/json" && content.Schema != nil {
			return *content, nil
		}
		if name == "text/plain" && content.Schema != nil {
			return *content, nil
		}
		if name == "text/html" && content.Schema != nil {
			return *content, nil
		}
	}
	return openapi3.MediaType{}, errors.New("response content not found")
}

func GetRequestContent(
	request openapi3.RequestBody,
) (types.RequestContent, error) {
	for name, content := range request.Content {
		if name == "application/json" && content.Schema != nil {
			return types.RequestContent{ContentType: name, Content: *content}, nil
		}
		if name == "application/x-www-form-urlencoded" && content.Schema != nil {
			return types.RequestContent{ContentType: name, Content: *content}, nil
		}
	}
	return types.RequestContent{}, errors.New("request content not found")
}

func ExtractRequestDataFromArgs(p graphql.ResolveParams, path string, httpMethod string, argToParam map[string]*openapi3.ParameterRef) string {
	endpoint := path
	queryString := []string{}

	for _, param := range argToParam {
		name := param.Value.Name
		value := p.Args[name]

		if value == nil {
			continue
		}

		if param.Value.In == "query" {
			v := utils.Serialize(value, name)
			queryString = append(queryString, v)
		} else if param.Value.In == "path" {
			toReplace := "{" + name + "}"
			newValue := utils.CastToString(value)
			endpoint = strings.Replace(endpoint, toReplace, newValue, 1)
		}
	}

	query := url.PathEscape(strings.Join(queryString, "&"))

	if len(query) > 0 {
		endpoint = endpoint + "?" + query
	}

	return endpoint
}
