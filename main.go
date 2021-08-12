package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"

	"openapi-to-graphql/oas_utils"
	typebuilder "openapi-to-graphql/type_builder"
	"openapi-to-graphql/types"
	"openapi-to-graphql/utils"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/graphql-go/graphql"
	"github.com/graphql-go/handler"
)

var oasPath = flag.String("path", "oas/1/spec.json", "Path to oas json spec")
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

			response, err := getSuccessResponse(operation.Responses)
			if err != nil {
				log.Print("Skipping " + operationName + "." + err.Error())
				continue
			}

			responseContent, err := getResponseContent(response)
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
					Type:        def.InputGraphqlType,
					Description: description,
				}

				argToParam[name] = parameter
			}

			var requestContentDefinition types.RequestBodyDefinition
			requestBody := operation.RequestBody

			if requestBody != nil {
				description := requestBody.Value.Description
				required := requestBody.Value.Required
				requestContent, err := getRequestContent(*requestBody.Value)
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
					Type:        def.InputGraphqlType,
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
			resolver := getResolver(serverUrl+path, method, argToParam, &requestContentDefinition)
			field := &graphql.Field{
				Name:        operationName,
				Description: operation.Description,
				Args:        args,
				Type:        def.GraphqlType,
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

func getResolver(path string, httpMethod string, argToParam map[string]*openapi3.ParameterRef, requestBodyDef *types.RequestBodyDefinition) func(p graphql.ResolveParams) (interface{}, error) {
	return func(p graphql.ResolveParams) (interface{}, error) {
		endpoint := extractRequestDataFromArgs(p, path, httpMethod, argToParam)

		requestBodyValue := p.Args[requestBodyDef.ArgumentName]

		body := oas_utils.Body{
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

func getRequestContent(
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
