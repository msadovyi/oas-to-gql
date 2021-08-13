package main

import (
	"flag"
	"log"
	"net/http"
	"reflect"

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

			response, err := oas_utils.GetSuccessResponse(operation.Responses)
			if err != nil {
				log.Print("Skipping " + operationName + "." + err.Error())
				continue
			}

			responseContent, err := oas_utils.GetResponseContent(response)
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
				requestContent, err := oas_utils.GetRequestContent(*requestBody.Value)
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
			resolver := oas_utils.GetResolver(client, serverUrl+path, method, argToParam, &requestContentDefinition)
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
