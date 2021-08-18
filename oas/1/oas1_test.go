package oas1

import (
	"bytes"
	"encoding/json"
	"log"
	"testing"

	oas_utils "openapi-to-graphql/oas_utils"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/graphql-go/graphql"
)

type TestCase struct {
	name         string
	query        string
	expectedJson string
}

var cases = []TestCase{
	findPets,
	findPetsWithFilters,
	findPetById,
	updatePetById,
	addPet,
	noResponseSchema,
	union1,
	union2,
	nestedParameter,
}

func TestCases(t *testing.T) {
	go StartTestServer("localhost:3000")

	public, err := openapi3.NewLoader().LoadFromFile("./spec.json")
	if err != nil {
		log.Fatalln(err)
	}

	config := oas_utils.TranslateToSchemaConfig(public)

	schema, err := graphql.NewSchema(config)
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			params := graphql.Params{Schema: schema, RequestString: tc.query}
			r := graphql.Do(params)

			if len(r.Errors) > 0 {
				t.Fatal(r.Errors)
			}

			got, err := json.Marshal(r)
			if err != nil {
				t.Fatalf("got: invalid JSON: %s", err)
			}
			want, err := formatJSON([]byte(tc.expectedJson))
			if err != nil {
				t.Fatalf("want: invalid JSON: %s", err)
			}

			if !bytes.Equal(got, want) {
				t.Logf("got:  %s", got)
				t.Logf("want: %s", want)
				t.Fail()
			}
		})
	}
}

func formatJSON(data []byte) ([]byte, error) {
	var v interface{}
	if err := json.Unmarshal(data, &v); err != nil {
		return nil, err
	}
	formatted, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return formatted, nil
}

var findPets = TestCase{
	name: "findPets",
	query: `{
		findPets {
			id
			name
			tag
		}
	}`,
	expectedJson: `{"data":{"findPets":[{"id":1,"name":"cat","tag":"cute"},{"id":2,"name":"dog","tag":"gentle"},{"id":3,"name":"dog2","tag":"dangerous"},{"id":4,"name":"wolf","tag":"dangerous"}]}}`,
}
var findPetsWithFilters = TestCase{
	name: "findPets with filters",
	query: `{
		findPets(limit: 1, tags: ["dangerous"]) {
			id
			name
			tag
		}
	}`,
	expectedJson: `{"data":{"findPets":[{"id":3,"name":"dog2","tag":"dangerous"}]}}`,
}
var findPetById = TestCase{
	name: "findPetById",
	query: `{
		findPetById(id: 1) {
			id
		}
	}`,
	expectedJson: `{"data":{"findPetById":{"id":1}}}`,
}
var updatePetById = TestCase{
	name: "updatePetById",
	query: `mutation {
		updatePet(id: 2, newPetInput: {
			tag:"tag",
			name:"name",
		}) {
			id
			name
			tag
		}
	}`,
	expectedJson: `{"data":{"updatePet":{"id":2,"name":"name","tag":"tag"}}}`,
}
var addPet = TestCase{
	name: "addPet",
	query: `mutation {
		addPet(newPetInput: {
			tag:"newTag",
			name:"newName",
		}) {
			id
			name
			tag
		}
	}`,
	expectedJson: `{"data":{"addPet":{"id":5,"name":"newName","tag":"newTag"}}}`,
}
var noResponseSchema = TestCase{
	name: "noResponseSchema",
	query: `{
		noResponseSchema
	}`,
	expectedJson: `{"data":{"noResponseSchema":{"branch":"ECE","float":10.5,"language":"C++","name":"Pikachu","particles":498}}}`,
}
var union1 = TestCase{
	name: "basicUnions",
	query: `mutation {
		breeds(breedsInput: { catBreed: true }) {
			__typename
			... on CatMember {
				catBreed
			}
		}
	}`,
	expectedJson: `{"data":{"breeds":{"__typename":"CatMember","catBreed":"Sphynx"}}}`,
}
var union2 = TestCase{
	name: "basicUnions",
	query: `mutation {
		breeds(breedsInput: { dogBreed: true }) {
			__typename
			... on DogMember {
				dogBreed
			}
		}
	}`,
	expectedJson: `{"data":{"breeds":{"__typename":"DogMember","dogBreed":"Labrador"}}}`,
}
var nestedParameter = TestCase{
	name: "nestedParameter",
	query: `{
		nestedReferenceInParameter(russianDoll: {
			name: "name"
			nestedDoll: {
				name: "name1",
				nestedDoll: {
					name: "name2"
				}
			}
		})
	}`,
	expectedJson: `{"data":{"nestedReferenceInParameter":"name,name1,name2"}}`,
}
