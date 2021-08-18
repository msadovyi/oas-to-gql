package typebuilder

import (
	"reflect"
	"strconv"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/graphql-go/graphql"
	"github.com/graphql-go/graphql/language/ast"
	"github.com/graphql-go/graphql/language/kinds"
	"github.com/jinzhu/copier"

	types "openapi-to-graphql/types"
	"openapi-to-graphql/utils"
)

var defs = make(map[string]*types.DataDefinition)

type UsedOT map[string]graphql.Type // graphql.Type can be field of types.DataDefinition struct schema, datadefs, subdefs and prefered gql type name

var usedOT = make(UsedOT)

func setUsedOT(def *types.DataDefinition) {
	usedOT[def.GraphQLTypeName] = def.GraphQLType
	usedOT[def.GraphQLInputTypeName] = def.InputGraphQLType
}

func assignGraphQLTypeToDefinition(def *types.DataDefinition) {
	if usedOT[def.GraphQLTypeName] != nil {
		def.GraphQLType = usedOT[def.GraphQLTypeName]
		def.InputGraphQLType = usedOT[def.GraphQLInputTypeName]
	} else if def.TargetGraphQLType == types.List {
		assignGraphQLTypeToDefinition(def.ListItemDefinitions)

		def.GraphQLType = graphql.NewList(def.ListItemDefinitions.GraphQLType)
		def.InputGraphQLType = graphql.NewList(def.ListItemDefinitions.InputGraphQLType)
		setUsedOT(def.ListItemDefinitions)
	} else if def.TargetGraphQLType == types.Object {
		def.GraphQLType = assignOt(def)
		def.InputGraphQLType = assignInputOt(def)
		setUsedOT(def)
	} else if def.TargetGraphQLType == types.Enum {
		def.GraphQLType = assignEnum(def)
		def.InputGraphQLType = def.GraphQLType
		setUsedOT(def)
	} else if def.TargetGraphQLType == types.Union {
		def.GraphQLType = assignUnion(def)
		// input type cannot be union
		def.InputGraphQLType = JSONScalar
		setUsedOT(def)
	} else if def.TargetGraphQLType == types.JSON {
		def.GraphQLType = JSONScalar
		def.InputGraphQLType = JSONScalar
	} else if def.TargetGraphQLType == types.String {
		def.GraphQLType = graphql.String
		def.InputGraphQLType = graphql.String
	} else if def.TargetGraphQLType == types.Integer {
		def.GraphQLType = graphql.Int
		def.InputGraphQLType = graphql.Int
	} else if def.TargetGraphQLType == types.Float {
		def.GraphQLType = graphql.Float
		def.InputGraphQLType = graphql.Float
	} else if def.TargetGraphQLType == types.Boolean {
		def.GraphQLType = graphql.Boolean
		def.InputGraphQLType = graphql.Boolean
	}

	if def.Required {
		def.GraphQLType = graphql.NewNonNull(def.GraphQLType)
		def.InputGraphQLType = graphql.NewNonNull(def.InputGraphQLType)
	}
}

func CreateDataDefinition(oas *openapi3.T, schemaRef *openapi3.SchemaRef, schemaNames types.SchemaNames, path string, required bool) *types.DataDefinition {
	preferredName := getPreferredName(schemaNames)
	targetGraphQLType := getTargetGraphQLType(schemaRef.Value)

	if targetGraphQLType == types.Union {
		preferredName += "Union"
	}

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
		TargetGraphQLType:    targetGraphQLType,
		Type:                 schemaRef.Value.Type,
	}

	if len(availableName) > 0 && (targetGraphQLType == types.Object || targetGraphQLType == types.Union || targetGraphQLType == types.Enum || targetGraphQLType == types.List) {
		defs[availableName] = &def
	}

	if targetGraphQLType == types.List {
		names := types.SchemaNames{
			FromRef: utils.GetRefName(schemaRef.Value.Items.Ref),
		}
		subDef := CreateDataDefinition(oas, schemaRef.Value.Items, names, path, false)
		def.ListItemDefinitions = subDef
	} else if targetGraphQLType == types.Object {
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
	} else if targetGraphQLType == types.Union {
		def.UnionDefinitions = createUnionDefinitions(oas, schemaRef, schemaNames, path, required)
	}

	assignGraphQLTypeToDefinition(&def)

	return &def
}

func createUnionDefinitions(oas *openapi3.T, schemaRef *openapi3.SchemaRef, schemaNames types.SchemaNames, path string, required bool) []*types.DataDefinition {
	schemaWithoutOneOf := &openapi3.SchemaRef{}
	copier.Copy(&schemaWithoutOneOf, &schemaRef)
	schemaWithoutOneOf.Value.OneOf = nil

	definitions := make([]*types.DataDefinition, 0)
	baseDefinition := CreateDataDefinition(oas, schemaWithoutOneOf, schemaNames, path, required)

	if baseDefinition.GraphQLType != nil {
		definitions = append(definitions, baseDefinition)
	}

	for _, oneOfSchema := range schemaRef.Value.OneOf {
		names := types.SchemaNames{
			FromRef:    utils.GetRefName(oneOfSchema.Ref),
			FromSchema: oneOfSchema.Value.Title,
			FromPath:   path,
		}
		memberTypeDefinition := CreateDataDefinition(oas, oneOfSchema, names, path, required)
		if memberTypeDefinition.GraphQLType != nil {
			definitions = append(definitions, memberTypeDefinition)
		}
	}

	return definitions
}

func getTargetGraphQLType(schema *openapi3.Schema) int {
	targetType := types.Unknown

	if len(schema.AllOf) > 0 && len(schema.OneOf) > 0 {
		targetType = types.JSON
	} else if len(schema.OneOf) > 0 {
		schemaWithoutOneOf := openapi3.Schema{}
		copier.Copy(&schemaWithoutOneOf, &schema)
		schemaWithoutOneOf.OneOf = nil

		baseType := getTargetGraphQLType(&schemaWithoutOneOf)

		memberSchemas := make([]*openapi3.Schema, 0)
		for _, member := range schema.OneOf {
			memberSchemas = append(memberSchemas, member.Value)
		}

		targetType = getOneOfTargetGraphQLType(&baseType, memberSchemas)
	} else if len(schema.Enum) > 0 {
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
	} else {
		targetType = types.JSON
	}

	return targetType
}

func getOneOfTargetGraphQLType(baseType *int, schemas []*openapi3.Schema) int {
	if len(schemas) == 0 {
		return *baseType
	}

	var memberTypes []int

	for _, schema := range schemas {
		memberType := getTargetGraphQLType(schema)
		memberTypes = append(memberTypes, memberType)
	}

	// if member types are different - it's json
	firstType := memberTypes[0]
	for _, t := range memberTypes {
		if firstType != t {
			return types.JSON
		}
	}

	if *baseType != types.Unknown {
		if *baseType != firstType {
			return types.JSON
		} else if *baseType == firstType && *baseType == types.Object {
			return types.Union
		}
	}
	if firstType == types.Object {
		return types.Union
	} else {
		return firstType
	}
}

func assignUnion(def *types.DataDefinition) graphql.Type {
	objectTypes := make([]*graphql.Object, 0)
	for _, d := range def.UnionDefinitions {
		objectTypes = append(objectTypes, d.GraphQLObject)
	}
	def.GraphQLType = graphql.NewUnion(graphql.UnionConfig{
		Name:        def.GraphQLTypeName,
		Description: def.Schema.Description,
		Types:       objectTypes,
		ResolveType: func(p graphql.ResolveTypeParams) *graphql.Object {
			// check p.Value properties and find corresponding def.GraphQLObject

			for _, d := range def.UnionDefinitions {
				isTargetObjectType := true

				for fieldName := range d.ObjectPropertiesDefinitions {
					value := p.Value.(map[string]interface{})

					if _, ok := value[fieldName]; !ok {
						isTargetObjectType = false
					}
				}

				if isTargetObjectType {
					return d.GraphQLObject
				}
			}
			// no GraphQLObject, returning nil equal to throwing gql error
			return nil
		},
	})

	return def.GraphQLType
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
	def.GraphQLObject = graphql.NewObject(graphql.ObjectConfig{
		Name: def.GraphQLTypeName,
		Fields: graphql.FieldsThunk(func() graphql.Fields {
			fields := graphql.Fields{}
			for fieldName, p := range def.ObjectPropertiesDefinitions {
				assignGraphQLTypeToDefinition(p)
				fields[fieldName] = &graphql.Field{Type: p.GraphQLType, Name: fieldName}
			}
			return fields
		}),
	})
	return def.GraphQLObject
}

func assignInputOt(def *types.DataDefinition) graphql.Type {
	return graphql.NewInputObject(
		graphql.InputObjectConfig{
			Name: def.GraphQLInputTypeName,
			Fields: graphql.InputObjectConfigFieldMapThunk(
				func() graphql.InputObjectConfigFieldMap {
					fields := graphql.InputObjectConfigFieldMap{}
					for fieldName, p := range def.ObjectPropertiesDefinitions {
						fields[fieldName] = &graphql.InputObjectFieldConfig{Type: p.InputGraphQLType}
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

func parseLiteral(astValue ast.Value) interface{} {
	kind := astValue.GetKind()

	switch kind {
	case kinds.StringValue:
		return astValue.GetValue()
	case kinds.BooleanValue:
		return astValue.GetValue()
	case kinds.IntValue:
		return astValue.GetValue()
	case kinds.FloatValue:
		return astValue.GetValue()
	case kinds.ObjectValue:
		obj := make(map[string]interface{})
		for _, v := range astValue.GetValue().([]*ast.ObjectField) {
			obj[v.Name.Value] = parseLiteral(v.Value)
		}
		return obj
	case kinds.ListValue:
		list := make([]interface{}, 0)
		for _, v := range astValue.GetValue().([]ast.Value) {
			list = append(list, parseLiteral(v))
		}
		return list
	default:
		return nil
	}
}

// JSON type
var JSONScalar = graphql.NewScalar(
	graphql.ScalarConfig{
		Name:        "JSON",
		Description: "The `JSON` scalar type represents JSON values as specified by [ECMA-404](http://www.ecma-international.org/publications/files/ECMA-ST/ECMA-404.pdf)",
		Serialize: func(value interface{}) interface{} {
			return value
		},
		ParseValue: func(value interface{}) interface{} {
			return value
		},
		ParseLiteral: parseLiteral,
	},
)
