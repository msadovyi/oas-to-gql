package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"

	typebuilder "openapi-to-graphql/type_builder"
	"openapi-to-graphql/types"
	"openapi-to-graphql/utils"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/graphql-go/graphql"
	"github.com/graphql-go/handler"
)

var oasPath = flag.String("path", "oas/1/spec.json", "Path to oas json spec")
var usedOT = make(typebuilder.UsedOT)
var client = http.Client{}

func main() {
	// also we have to translate openapi2 to openapi3
	public, err := openapi3.NewLoader().LoadFromFile(*oasPath)
	if err != nil {
		log.Fatalln(err)
	}

	config := translateToSchemaConfig(public)

	schema, err := graphql.NewSchema(config)
	if err != nil {
		log.Fatal(err)
	}

	h := handler.New(&handler.Config{
		Schema:     &schema,
		Pretty:     true,
		GraphiQL:   false,
		Playground: true,
	})

	http.Handle("/", h)

	log.Print("Server is listening port 8080")
	err = http.ListenAndServe(":8080", nil)
	if err != nil {
		log.Panic("Error when starting the http server", err)
	}
}

func translateToSchemaConfig(public *openapi3.T) graphql.SchemaConfig {
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
			operationName := utils.ToCamelCase(operation.OperationID)

			httpMethod, err := types.GetHttpMethod(method)
			if err != nil {
				log.Print("Skipping " + operationName + "." + err.Error())
				continue
			}

			response, err := getSuccessResponse(operation.Responses)
			if err != nil {
				log.Print("Skipping " + operationName + "." + err.Error())
				continue
			}

			content, err := getResponseContent(response)
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
				name := p.Name
				description := p.Description
				graphqlType := typebuilder.GetGraphQLType(public, p.Schema, usedOT, p.Required, true)

				args[name] = &graphql.ArgumentConfig{
					Type:        graphqlType,
					Description: description,
				}

				argToParam[name] = parameter
			}

			var requestBodyArgName string
			requestBody := operation.RequestBody

			if requestBody != nil {
				description := requestBody.Value.Description
				required := requestBody.Value.Required
				content, err := getRequestContent(*requestBody.Value)
				if err != nil {
					panic(err)
				}
				graphqlType := typebuilder.GetGraphQLType(public, content.Schema, usedOT, required, true)

				requestBodyArgName, err = utils.Sanitize(graphqlType.Name())
				if err != nil {
					panic(err)
				}

				args[requestBodyArgName] = &graphql.ArgumentConfig{ // should be astraction, with simple data definition, not argument config
					Type:        graphqlType,
					Description: description,
				}
				/*
					type DataDefinition struct {
						Name        string      `json:"name"`
						GraphqlArgumentConfig  graphql.ArgumentConfig `json:"graphql_argument"`
						Fields      interface{} `json:"fields"`
						GraphqlType
						Schema
						Type
						Required ...
					}
				*/
			}

			fieldType := typebuilder.GetGraphQLType(public, content.Schema, usedOT, false, false)
			resolver := getResolver(serverUrl+path, method, argToParam, requestBodyArgName)
			field := &graphql.Field{
				Name:        operationName,
				Description: operation.Description,
				Args:        args,
				Type:        fieldType,
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

	return graphql.SchemaConfig{
		Query: graphql.NewObject(graphql.ObjectConfig{
			Name:   "Query",
			Fields: queryFields,
		}),
		Mutation: graphql.NewObject(graphql.ObjectConfig{
			Name:   "Mutation",
			Fields: mutationFields,
		}),
	}
}

func getResolver(path string, httpMethod string, argToParam map[string]*openapi3.ParameterRef, requestBodyArgName string) func(p graphql.ResolveParams) (interface{}, error) {
	return func(p graphql.ResolveParams) (interface{}, error) {
		endpoint := extractRequestDataFromArgs(p, path, httpMethod, argToParam)

		requestBodyValue := p.Args[requestBodyArgName]
		var body io.Reader

		if requestBodyValue != nil {
			body = marshalRequestBody(requestBodyValue)
		}

		request, err := http.NewRequest(strings.ToUpper(httpMethod), endpoint, body)
		if err != nil {
			return nil, err
		}
		request.Header.Set("Content-Type", "application/json") // could be another type, depends on DataDefinition..

		response, err := client.Do(request)
		if err != nil {
			return nil, err
		}

		defer response.Body.Close()

		responseBody, err := ioutil.ReadAll(response.Body)
		if err != nil {
			return nil, err
		}
		var data interface{} // also could be a string, have to parse response schema and try every case

		json.Unmarshal(responseBody, &data)

		if response.StatusCode >= 400 {
			err := fmt.Errorf("StatusCode: %v. Status: %v. Response body: %v", response.StatusCode, response.Status, data)
			return nil, err
		}

		return data, nil
	}
}

func getSuccessResponse(
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

func getResponseContent(
	response openapi3.ResponseRef,
) (openapi3.MediaType, error) {
	for name, content := range response.Value.Content {
		if name == "application/json" && content != nil { // also can be xml, etc..
			return *content, nil
		}
	}
	return openapi3.MediaType{}, errors.New("response content not found")
}

func getRequestContent(
	request openapi3.RequestBody,
) (openapi3.MediaType, error) {
	for name, content := range request.Content {
		if name == "application/json" && content != nil {
			return *content, nil
		}
	}
	return openapi3.MediaType{}, errors.New("request content not found")
}

func extractRequestDataFromArgs(p graphql.ResolveParams, path string, httpMethod string, argToParam map[string]*openapi3.ParameterRef) string {
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

func marshalRequestBody(data interface{}) io.Reader {
	var jsonStr, err = json.Marshal(data)
	if err != nil {
		panic(err)
	}

	return bytes.NewBuffer([]byte(jsonStr))
}
