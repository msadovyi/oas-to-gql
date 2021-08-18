package oas1

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

type Pet struct {
	Id   int    `json:"id"`
	Name string `json:"name"`
	Tag  string `json:"tag"`
}

var cat = Pet{
	Id:   1,
	Name: "cat",
	Tag:  "cute",
}
var dog = Pet{
	Id:   2,
	Name: "dog",
	Tag:  "gentle",
}
var dog2 = Pet{
	Id:   3,
	Name: "dog2",
	Tag:  "dangerous",
}
var wolf = Pet{
	Id:   4,
	Name: "wolf",
	Tag:  "dangerous",
}

var pets = []Pet{
	cat,
	dog,
	dog2,
	wolf,
}

type RussianDoll struct {
	Name       string       `json:"name"`
	NestedDoll *RussianDoll `json:"nestedDoll"`
}

func StartTestServer(addr string) {
	router := gin.New()

	router.POST("/breeds", getBreedsHandler)
	router.GET("/no-response-schema", noResponseSchemaHandler)
	router.GET("/pets", getPetsHandler)
	router.POST("/pets", addPetHandler)
	router.GET("/pets/:id", getPetByIdHandler)
	router.PUT("/pets/:id", updatePetByIdHandler)
	router.GET("nestedReferenceInParameter", nestedReferenceInParameterHandler)
	router.Run(addr)
}

func nestedReferenceInParameterHandler(c *gin.Context) {
	russianDoll := c.Request.URL.Query()
	names := make([]string, 0)
	print(russianDoll)
	for _, name := range russianDoll {
		names = append(names, name[0])
	}
	sort.Strings(names)
	c.String(http.StatusOK, strings.Join(names, ","))
}

func getBreedsHandler(c *gin.Context) {
	var body struct {
		CatBreed bool `json:"catBreed"`
		DogBreed bool `json:"dogBreed"`
	}
	err := json.NewDecoder(c.Request.Body).Decode(&body)
	if err != nil {
		c.JSON(http.StatusBadRequest, "Can't decode body")
		return
	}

	if body.CatBreed {
		data := struct {
			CatBreed string `json:"catBreed"`
		}{
			CatBreed: "Sphynx",
		}

		c.JSON(http.StatusOK, data)
		return
	}

	data := struct {
		DogBreed string `json:"dogBreed"`
	}{
		DogBreed: "Labrador",
	}

	c.JSON(http.StatusOK, data)
}

func noResponseSchemaHandler(c *gin.Context) {
	data := struct {
		Name      string  `json:"name"`
		Branch    string  `json:"branch"`
		Language  string  `json:"language"`
		Particles int     `json:"particles"`
		Float     float32 `json:"float"`
	}{
		Name:      "Pikachu",
		Branch:    "ECE",
		Language:  "C++",
		Particles: 498,
		Float:     10.5,
	}

	c.JSON(http.StatusOK, data)
}

func addPetHandler(c *gin.Context) {
	var pData Pet
	err := json.NewDecoder(c.Request.Body).Decode(&pData)
	if err != nil {
		c.JSON(http.StatusBadRequest, "Can't decode body")
		return
	}
	pet := Pet{
		Id:   len(pets) + 1,
		Name: pData.Name,
		Tag:  pData.Tag,
	}
	pets = append(pets, pet)

	c.JSON(http.StatusOK, pet)
}

func updatePetByIdHandler(c *gin.Context) {
	idStr, ok := c.Params.Get("id")
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Id was not provided",
		})
		return
	}
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": err,
		})
		return
	}

	found := Pet{}
	for _, pet := range pets {
		if pet.Id == id {
			found = pet
			break
		}
	}

	if found == (Pet{}) {
		c.JSON(http.StatusBadRequest, "Pet not found")
		return
	}

	var pData Pet
	err = json.NewDecoder(c.Request.Body).Decode(&pData)
	if err != nil {
		c.JSON(http.StatusBadRequest, "Can't decode body")
		return
	}
	found.Name = pData.Name
	if len(pData.Tag) > 0 {
		found.Tag = pData.Tag
	}

	c.JSON(http.StatusOK, found)
}

func getPetByIdHandler(c *gin.Context) {
	idStr, ok := c.Params.Get("id")
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Id was not provided",
		})
		return
	}
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": err,
		})
		return
	}

	var found Pet
	for _, pet := range pets {
		if pet.Id == id {
			found = pet
			break
		}
	}
	if found == (Pet{}) {
		c.JSON(http.StatusBadRequest, "Pet not found")
		return
	}

	c.JSON(http.StatusOK, found)
}

func getPetsHandler(c *gin.Context) {
	queryParams := c.Request.URL.Query()
	filtered := []Pet{}
	limit := queryParams.Get("limit")

	c.Request.ParseForm() // parses request body and query and stores result in r.Form
	var tags []string
	for i := 0; ; i++ {
		key := fmt.Sprintf("tags[%d]", i)
		values := c.Request.Form[key] // form values are a []string
		if len(values) == 0 {
			// no more values
			break
		}
		tags = append(tags, values[i])
		i++
	}

	if len(tags) > 0 {
		for _, pet := range pets {
			shouldBeAdded := false
			for _, v := range tags {
				if v == pet.Tag {
					shouldBeAdded = true
				}
			}
			if shouldBeAdded {
				filtered = append(filtered, pet)
			}
		}
	} else {
		filtered = pets
	}
	if len(limit) > 0 {
		l, err := strconv.Atoi(limit)
		if err != nil {
			l = 0
		}
		filtered = filtered[0:l]
	}

	data, _ := json.Marshal(filtered)
	c.Data(http.StatusOK, "application/json", data)
}
