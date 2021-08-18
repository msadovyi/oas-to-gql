package oas_utils

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	typebuilder "openapi-to-graphql/type_builder"
	"openapi-to-graphql/types"
	"openapi-to-graphql/utils"
	"reflect"
	"strconv"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/graphql-go/graphql"
)

var client = http.Client{}

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

func TranslateToSchemaConfig(public *openapi3.T) graphql.SchemaConfig {
	serverUrl := utils.GetServerUrl(public)

	queryFields := graphql.Fields{}
	mutationFields := graphql.Fields{}

	for path, pathItem := range public.Paths {
		for _, method := range types.HttpMethodsList() {
			// iterate through struct fields
			operation, ok := reflect.Indirect(reflect.ValueOf(pathItem)).FieldByName(method).Interface().(*openapi3.Operation)
			if !ok || operation == nil {
				continue
			}
			operationName := operation.OperationID
			if len(operationName) == 0 {
				operationName = utils.InferResourceNameFromPath(path)
			}
			operationName = utils.ToCamelCase(operationName)

			httpMethod, err := types.GetHttpMethod(method)
			if err != nil {
				log.Print("Skipping " + operationName + "." + err.Error())
				continue
			}

			response, err := GetSuccessResponse(operation.Responses)
			if err != nil {
				log.Print("Skipping " + operationName + "." + err.Error())
				continue
			}

			responseContent, err := GetResponseContent(response)
			if err != nil {
				log.Print("Skipping " + operationName + "." + err.Error())
				continue
			}

			operationType := types.Query
			if httpMethod != types.HttpMethod.Get {
				operationType = types.Mutation
			}

			args := graphql.FieldConfigArgument{}
			// map of arg sane name to parameter
			argToParam := make(map[string]*openapi3.ParameterRef)

			for _, parameter := range operation.Parameters {
				p := parameter.Value
				name := utils.ToCamelCase(p.Name)
				description := p.Description

				names := types.SchemaNames{
					FromSchema: name,
				}
				schema := p.Schema
				if schema == nil {
					schema = p.Content["application/json"].Schema
				}
				if schema == nil {
					log.Print("Skipping " + operationName + "." + "Parameter schema not found")
					continue
				}
				def := typebuilder.CreateDataDefinition(public, schema, names, path, p.Required)

				args[name] = &graphql.ArgumentConfig{
					Type:        def.InputGraphQLType,
					Description: description,
				}

				argToParam[name] = parameter
			}

			var requestContentDefinition types.RequestBodyDefinition
			requestBody := operation.RequestBody

			if requestBody != nil {
				description := requestBody.Value.Description
				required := requestBody.Value.Required
				requestContent, err := GetRequestContent(*requestBody.Value)
				if err != nil {
					log.Print("Skipping " + operationName + "." + err.Error())
					continue
				}

				schemaNames := types.SchemaNames{
					FromSchema: requestContent.Content.Schema.Value.Title,
					FromRef:    utils.GetRefName(requestContent.Content.Schema.Ref),
					FromPath:   utils.InferResourceNameFromPath(path),
				}

				def := typebuilder.CreateDataDefinition(public, requestContent.Content.Schema, schemaNames, path, required)

				argumentName := utils.ToCamelCase(def.GraphQLInputTypeName)

				args[argumentName] = &graphql.ArgumentConfig{ // should be astraction, with simple data definition, not argument config
					Type:        def.InputGraphQLType,
					Description: description,
				}
				requestContentDefinition = types.RequestBodyDefinition{
					ContentType:    requestContent.ContentType,
					ArgumentName:   argumentName,
					DataDefinition: def,
				}
			}

			schemaNames := types.SchemaNames{
				FromSchema: responseContent.Schema.Value.Title,
				FromRef:    utils.GetRefName(responseContent.Schema.Ref),
				FromPath:   utils.InferResourceNameFromPath(path),
			}

			def := typebuilder.CreateDataDefinition(public, responseContent.Schema, schemaNames, path, false)
			resolver := GetResolver(client, serverUrl+path, method, argToParam, &requestContentDefinition)
			field := &graphql.Field{
				Name:        operationName,
				Description: operation.Description,
				Args:        args,
				Type:        def.GraphQLType,
				Resolve:     resolver,
			}

			if operationType == types.Query {
				queryFields[operationName] = field
			} else {
				mutationFields[operationName] = field
			}

			log.Print("Added field: " + operationName)
		}
		log.Print("Path processed: " + path)
	}

	config := graphql.SchemaConfig{}

	if len(mutationFields) > 0 {
		config.Mutation = graphql.NewObject(graphql.ObjectConfig{
			Name:   "Mutation",
			Fields: mutationFields,
		})
	}
	if len(queryFields) > 0 {
		config.Query = graphql.NewObject(graphql.ObjectConfig{
			Name:   "Query",
			Fields: queryFields,
		})
	}

	return config
}
