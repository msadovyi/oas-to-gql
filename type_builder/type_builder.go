package typebuilder

import (
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/graphql-go/graphql"

	types "openapi-to-graphql/types"
	"openapi-to-graphql/utils"
)

var defs = make(map[string]*DataDefinition)

type SchemaNames struct {
	FromRef    string
	FromSchema string
	FromPath   string
}

type DataDefinition struct {
	Path                        string
	OAS                         *openapi3.T
	SchemaRef                   *openapi3.SchemaRef
	Schema                      *openapi3.Schema
	Names                       SchemaNames
	PreferredName               string
	Type                        string
	TargetGraphqlType           int
	Required                    bool
	ObjectPropertiesDefinitions map[string]*DataDefinition
	ListItemDefinitions         *DataDefinition
	GraphqlType                 graphql.Type
	InputGraphqlType            graphql.Type
}

type UsedOT map[string]graphql.Type // graphql.Type can be field of DataDefinition struct schema, datadefs, subdefs and prefered gql type name

var usedOT = make(UsedOT)

func setUsedOT(def *DataDefinition) {
	usedOT[def.GraphqlType.Name()] = def.GraphqlType
	usedOT[def.InputGraphqlType.Name()] = def.InputGraphqlType
}

func assignGraphQLTypeToDefinition(def *DataDefinition) {
	if usedOT[def.PreferredName] != nil {
		def.GraphqlType = usedOT[def.PreferredName]
		def.InputGraphqlType = usedOT[def.PreferredName+"Input"]
	} else if def.TargetGraphqlType == types.List {
		assignGraphQLTypeToDefinition(def.ListItemDefinitions)

		def.GraphqlType = graphql.NewList(def.ListItemDefinitions.GraphqlType)
		def.InputGraphqlType = graphql.NewList(def.ListItemDefinitions.InputGraphqlType)

		setUsedOT(def.ListItemDefinitions)
	} else if def.TargetGraphqlType == types.Object {
		def.GraphqlType = assignOt(def)
		def.InputGraphqlType = assignInputOt(def)

		setUsedOT(def)
	} else if def.TargetGraphqlType == types.String {
		def.GraphqlType = graphql.String
		def.InputGraphqlType = graphql.String
	} else if def.TargetGraphqlType == types.Integer {
		def.GraphqlType = graphql.Int
		def.InputGraphqlType = graphql.Int
	} else if def.TargetGraphqlType == types.Float {
		def.GraphqlType = graphql.Float
		def.InputGraphqlType = graphql.Float
	} else if def.TargetGraphqlType == types.Boolean {
		def.GraphqlType = graphql.Boolean
		def.InputGraphqlType = graphql.Boolean
	}

	if def.Required {
		def.GraphqlType = graphql.NewNonNull(def.GraphqlType)
		def.InputGraphqlType = graphql.NewNonNull(def.InputGraphqlType)
	}
}

func CreateDataDefinition(oas *openapi3.T, schemaRef *openapi3.SchemaRef, schemaNames SchemaNames, path string, required bool) *DataDefinition {
	preferredName := GetPreferredName(schemaNames)

	if defs[preferredName] != nil {
		return defs[preferredName]
	}

	def := DataDefinition{
		Path:              path,
		OAS:               oas,
		SchemaRef:         schemaRef,
		Schema:            schemaRef.Value,
		Names:             schemaNames,
		PreferredName:     preferredName,
		Required:          required,
		TargetGraphqlType: getTargetGraphqlType(schemaRef.Value),
		Type:              schemaRef.Value.Type,
	}

	defs[preferredName] = &def

	if def.TargetGraphqlType == types.List {
		names := SchemaNames{
			FromRef: utils.GetRefName(schemaRef.Value.Items.Ref),
		}
		subDef := CreateDataDefinition(oas, schemaRef.Value.Items, names, path, false)
		def.ListItemDefinitions = subDef
	} else if def.TargetGraphqlType == types.Object {
		objectDefinitions := make(map[string]*DataDefinition)
		schemas := make([]*openapi3.SchemaRef, 0)

		schemas = append(schemas, schemaRef)

		if schemaRef.Value.AllOf != nil && len(schemaRef.Value.AllOf) > 0 {
			for _, allOfSchema := range schemaRef.Value.AllOf {
				schemas = append(schemas, allOfSchema)
			}
		}

		for _, schema := range schemas {
			for fieldName, value := range schema.Value.Properties {
				names := SchemaNames{
					FromRef:    utils.GetRefName(value.Ref),
					FromSchema: value.Value.Title,
				}
				required := utils.Contains(schemaRef.Value.Required, fieldName)
				subDefinition := CreateDataDefinition(oas, value, names, path, required)
				objectDefinitions[fieldName] = subDefinition
			}
		}

		def.ObjectPropertiesDefinitions = objectDefinitions
	}

	assignGraphQLTypeToDefinition(&def)

	return &def
}

func getTargetGraphqlType(schema *openapi3.Schema) int {
	targetType := types.Unknown
	if len(schema.AllOf) > 0 || schema.Type == "object" {
		targetType = types.Object
	} else if schema.Type == "array" {
		targetType = types.List
	} else if schema.Type == "string" {
		targetType = types.String
	} else if schema.Type == "integer" {
		targetType = types.Integer
	} else if schema.Type == "float" {
		targetType = types.Float
	} else if schema.Type == "boolean" {
		targetType = types.Boolean
	}

	return targetType
}

func assignOt(def *DataDefinition) graphql.Type {
	return graphql.NewObject(
		graphql.ObjectConfig{
			Name: def.PreferredName,
			Fields: graphql.FieldsThunk(func() graphql.Fields {
				fields := graphql.Fields{}
				for fieldName, p := range def.ObjectPropertiesDefinitions {
					assignGraphQLTypeToDefinition(p)
					fields[fieldName] = &graphql.Field{Type: p.GraphqlType, Name: fieldName}
				}
				return fields
			}),
		},
	)
}

func assignInputOt(def *DataDefinition) graphql.Type {
	return graphql.NewInputObject(
		graphql.InputObjectConfig{
			Name: def.PreferredName + "Input",
			Fields: graphql.InputObjectConfigFieldMapThunk(
				func() graphql.InputObjectConfigFieldMap {
					fields := graphql.InputObjectConfigFieldMap{}
					for fieldName, p := range def.ObjectPropertiesDefinitions {
						fields[fieldName] = &graphql.InputObjectFieldConfig{Type: p.InputGraphqlType}
					}
					return fields
				},
			),
		},
	)
}

func GetPreferredName(names SchemaNames) string {
	preferredName := ""

	if len(names.FromRef) > 0 {
		preferredName = names.FromRef
	} else if len(names.FromSchema) > 0 {
		preferredName = names.FromSchema
	} else if len(names.FromPath) > 0 {
		preferredName = names.FromPath
	}

	return utils.ToPascalCase(preferredName)
}
