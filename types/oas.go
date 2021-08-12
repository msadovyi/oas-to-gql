package types

import (
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/graphql-go/graphql"
)

type RequestBodyDefinition struct {
	ContentType    string
	ArgumentName   string
	DataDefinition *DataDefinition
}

type RequestContent struct {
	ContentType string
	Content     openapi3.MediaType
}

type DataDefinition struct {
	Path                        string
	OAS                         *openapi3.T
	SchemaRef                   *openapi3.SchemaRef
	Schema                      *openapi3.Schema
	Names                       SchemaNames
	GraphQLTypeName             string
	GraphQLInputTypeName        string
	Type                        string
	TargetGraphqlType           int
	Required                    bool
	ObjectPropertiesDefinitions map[string]*DataDefinition
	ListItemDefinitions         *DataDefinition
	GraphqlType                 graphql.Type
	InputGraphqlType            graphql.Type
}

type SchemaNames struct {
	FromRef    string
	FromSchema string
	FromPath   string
}
