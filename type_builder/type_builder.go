package typebuilder

import (
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/graphql-go/graphql"

	"openapi-to-graphql/utils"
)

type UsedOT map[string]graphql.Type // graphql.Type can be field of DataDefinition struct schema, datadefs, subdefs and prefered gql type name

func GetGraphQLType(oas *openapi3.T, schema *openapi3.SchemaRef, usedOT UsedOT, required bool, isInputType bool) graphql.Type {
	// should accept iteration argument, for breaking recoursive
	var graphqlType graphql.Type
	schemaValue := schema.Value
	schemaName := schema.Value.Title

	if len(schema.Ref) > 0 {
		schemaName = utils.GetRefName(schema.Ref)
	}

	if isInputType && len(schemaName) > 0 {
		schemaName += "Input"
	}

	if usedOT[schemaName] != nil {
		graphqlType = usedOT[schemaName]
	} else if len(schemaValue.AllOf) > 0 {
		// also can be anyof, oneof...
		graphqlType = createOrReuseAllOf(oas, schema, usedOT, schemaName, required, isInputType)
	} else if schemaValue.Type == "array" {
		graphqlType = createOrReuseList(oas, schema, usedOT, required, isInputType)
	} else if schemaValue.Type == "object" {
		graphqlType = createOrReuseOt(oas, schema, usedOT, schemaName, required, isInputType)
	} else if schemaValue.Type == "string" {
		graphqlType = graphql.String
	} else if schemaValue.Type == "integer" {
		graphqlType = graphql.Int
	} else if schemaValue.Type == "float" {
		graphqlType = graphql.Float
	} else if schemaValue.Type == "boolean" {
		graphqlType = graphql.Boolean
	}

	if required {
		graphqlType = graphql.NewNonNull(graphqlType)
	}

	return graphqlType
}

func createOrReuseList(oas *openapi3.T, schema *openapi3.SchemaRef, usedOT UsedOT, required bool, isInputType bool) graphql.Type {
	items := schema.Value.Items
	itemType := GetGraphQLType(oas, items, usedOT, utils.Contains(items.Value.Required, items.Value.Title), isInputType)
	listType := graphql.NewList(itemType)

	return listType
}

func createOrReuseOt(oas *openapi3.T, schema *openapi3.SchemaRef, usedOT UsedOT, name string, required bool, isInputType bool) graphql.Type {
	var ObjectType graphql.Type

	if isInputType {
		ObjectType = createInputOt(oas, schema, usedOT, name, required)
	} else {
		ObjectType = createOt(oas, schema, usedOT, name, required)
	}

	usedOT[name] = ObjectType

	return ObjectType
}

func createOt(oas *openapi3.T, schema *openapi3.SchemaRef, usedOT UsedOT, name string, required bool) graphql.Type {
	fields := graphql.Fields{}

	for fieldName, p := range schema.Value.Properties {
		fieldType := GetGraphQLType(oas, p, usedOT, utils.Contains(schema.Value.Required, fieldName), false)

		fields[fieldName] = &graphql.Field{Type: fieldType, Name: fieldName}
	}

	return graphql.NewObject(
		graphql.ObjectConfig{
			Name:   name,
			Fields: fields,
		},
	)
}

func createInputOt(oas *openapi3.T, schema *openapi3.SchemaRef, usedOT UsedOT, name string, required bool) graphql.Type {
	fields := graphql.InputObjectConfigFieldMap{}

	for fieldName, p := range schema.Value.Properties {
		fieldType := GetGraphQLType(oas, p, usedOT, utils.Contains(schema.Value.Required, fieldName), true)

		fields[fieldName] = &graphql.InputObjectFieldConfig{Type: fieldType}
	}

	return graphql.NewInputObject(
		graphql.InputObjectConfig{
			Name:   name,
			Fields: fields,
		},
	)
}

func createOrReuseAllOf(oas *openapi3.T, schema *openapi3.SchemaRef, usedOT UsedOT, name string, required bool, isInputType bool) graphql.Type {
	var ObjectType graphql.Type

	if isInputType {
		ObjectType = createAllOfInput(oas, schema, usedOT, name, required, true)
	} else {
		ObjectType = createAllOf(oas, schema, usedOT, name, required, false)
	}

	usedOT[name] = ObjectType

	return ObjectType
}

func createAllOf(oas *openapi3.T, schema *openapi3.SchemaRef, usedOT UsedOT, name string, required bool, isInputType bool) graphql.Type {
	// iterate through properties and get type
	fields := graphql.Fields{}

	for _, s := range schema.Value.AllOf {
		schemaValue := s.Value

		for fieldName, p := range schemaValue.Properties {
			fieldType := GetGraphQLType(oas, p, usedOT, utils.Contains(schemaValue.Required, fieldName), isInputType)
			description := p.Value.Description

			fields[fieldName] = &graphql.Field{
				Type:        fieldType,
				Name:        fieldName,
				Description: description,
			}
		}
	}

	var ObjectType = graphql.NewObject(
		graphql.ObjectConfig{
			Name:   name,
			Fields: fields,
		},
	)

	usedOT[name] = ObjectType

	return ObjectType
}

func createAllOfInput(oas *openapi3.T, schema *openapi3.SchemaRef, usedOT UsedOT, name string, required bool, isInputType bool) graphql.Type {
	// iterate through properties and get type
	fields := graphql.InputObjectConfigFieldMap{}

	for _, s := range schema.Value.AllOf {
		schemaValue := s.Value

		for fieldName, p := range schemaValue.Properties {
			fieldType := GetGraphQLType(oas, p, usedOT, utils.Contains(schemaValue.Required, fieldName), isInputType)
			description := p.Value.Description

			fields[fieldName] = &graphql.InputObjectFieldConfig{Type: fieldType, Description: description}
		}
	}

	var ObjectType = graphql.NewObject(
		graphql.ObjectConfig{
			Name:   name,
			Fields: fields,
		},
	)

	usedOT[name] = ObjectType

	return ObjectType
}
