package main

import (
	"flag"
	"log"
	"net/http"
	"openapi-to-graphql/oas_utils"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/graphql-go/graphql"
	"github.com/graphql-go/handler"
)

var oasPath = flag.String("path", "oas/1/spec.json", "Path to oas json spec")

func main() {
	// also we have to translate openapi2 to openapi3
	public, err := openapi3.NewLoader().LoadFromFile(*oasPath)
	if err != nil {
		log.Fatalln(err)
	}

	config := oas_utils.TranslateToSchemaConfig(public)

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
