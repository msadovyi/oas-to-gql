package typebuilder

import (
	"reflect"
	"strconv"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/graphql-go/graphql"

	types "openapi-to-graphql/types"
	"openapi-to-graphql/utils"
)

var defs = make(map[string]*types.DataDefinition)

type UsedOT map[string]graphql.Type // graphql.Type can be field of types.DataDefinition struct schema, datadefs, subdefs and prefered gql type name

var usedOT = make(UsedOT)

func setUsedOT(def *types.DataDefinition) {
	usedOT[def.GraphQLTypeName] = def.GraphqlType
	usedOT[def.GraphQLInputTypeName] = def.InputGraphqlType
}

func assignGraphQLTypeToDefinition(def *types.DataDefinition) {
	if usedOT[def.GraphQLTypeName] != nil {
		def.GraphqlType = usedOT[def.GraphQLTypeName]
		def.InputGraphqlType = usedOT[def.GraphQLInputTypeName]
	} else if def.TargetGraphqlType == types.List {
		assignGraphQLTypeToDefinition(def.ListItemDefinitions)

		def.GraphqlType = graphql.NewList(def.ListItemDefinitions.GraphqlType)
		def.InputGraphqlType = graphql.NewList(def.ListItemDefinitions.InputGraphqlType)

		setUsedOT(def.ListItemDefinitions)
	} else if def.TargetGraphqlType == types.Object {
		def.GraphqlType = assignOt(def)
		def.InputGraphqlType = assignInputOt(def)

		setUsedOT(def)
	} else if def.TargetGraphqlType == types.Enum {
		def.GraphqlType = assignEnum(def)
		def.InputGraphqlType = def.GraphqlType

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

func CreateDataDefinition(oas *openapi3.T, schemaRef *openapi3.SchemaRef, schemaNames types.SchemaNames, path string, required bool) *types.DataDefinition {
	preferredName := getPreferredName(schemaNames)
	availableName := getAvailableTypeName(preferredName, preferredName, schemaRef.Value, 1)

	if defs[availableName] != nil {
		return defs[availableName]
	}

	def := types.DataDefinition{
		Path:                 path,
		OAS:                  oas,
		SchemaRef:            schemaRef,
		Schema:               schemaRef.Value,
		Names:                schemaNames,
		GraphQLTypeName:      availableName,
		GraphQLInputTypeName: availableName + "Input",
		Required:             schemaRef.Value.Nullable || required,
		TargetGraphqlType:    getTargetGraphqlType(schemaRef.Value),
		Type:                 schemaRef.Value.Type,
	}

	if len(availableName) > 0 {
		defs[availableName] = &def
	}

	if def.TargetGraphqlType == types.List {
		names := types.SchemaNames{
			FromRef: utils.GetRefName(schemaRef.Value.Items.Ref),
		}
		subDef := CreateDataDefinition(oas, schemaRef.Value.Items, names, path, false)
		def.ListItemDefinitions = subDef
	} else if def.TargetGraphqlType == types.Object {
		objectDefinitions := make(map[string]*types.DataDefinition)
		schemas := make([]*openapi3.SchemaRef, 0)

		schemas = append(schemas, schemaRef)

		if schemaRef.Value.AllOf != nil && len(schemaRef.Value.AllOf) > 0 {
			for _, allOfSchema := range schemaRef.Value.AllOf {
				schemas = append(schemas, allOfSchema)
			}
		}

		for _, schema := range schemas {
			for fieldName, value := range schema.Value.Properties {
				names := types.SchemaNames{
					FromRef:    utils.GetRefName(value.Ref),
					FromSchema: value.Value.Title,
				}
				if len(names.FromSchema) == 0 {
					names.FromSchema = utils.ToPascalCase(fieldName)
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
	if len(schema.Enum) > 0 {
		targetType = types.Enum
	} else if len(schema.AllOf) > 0 || schema.Type == "object" {
		targetType = types.Object
	} else if schema.Type == "array" {
		targetType = types.List
	} else if schema.Type == "string" {
		targetType = types.String
	} else if schema.Type == "integer" {
		targetType = types.Integer
	} else if schema.Type == "number" {
		targetType = types.Float
	} else if schema.Type == "boolean" {
		targetType = types.Boolean
	}

	return targetType
}

func assignEnum(def *types.DataDefinition) graphql.Type {
	enumConfigMap := graphql.EnumValueConfigMap{}

	for _, v := range def.Schema.Enum {
		value := utils.CastToString(v)
		if len(value) > 0 {
			enumConfigMap[strings.ToUpper(value)] = &graphql.EnumValueConfig{
				Value: value,
			}
		}
	}

	return graphql.NewEnum(graphql.EnumConfig{
		Name:   def.GraphQLTypeName,
		Values: enumConfigMap,
	})
}

func assignOt(def *types.DataDefinition) graphql.Type {
	return graphql.NewObject(
		graphql.ObjectConfig{
			Name: def.GraphQLTypeName,
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

func assignInputOt(def *types.DataDefinition) graphql.Type {
	return graphql.NewInputObject(
		graphql.InputObjectConfig{
			Name: def.GraphQLInputTypeName,
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

func getPreferredName(names types.SchemaNames) string {
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

// Returns available name of gql type. If type already exists returns preferredName + "i"
func getAvailableTypeName(preferredName string, previousName string, schema *openapi3.Schema, i int) string {
	if defs[preferredName] != nil {
		// if schemas are deep equal reuse name
		if reflect.DeepEqual(defs[preferredName].Schema, schema) {
			return preferredName
		} else {
			i += 1
			// add number to the end of string and check again. We need previous name to do not mutate current
			preferredName = previousName + strconv.Itoa(i)
			return getAvailableTypeName(preferredName, previousName, schema, i)
		}
	} else {
		return preferredName
	}
}
