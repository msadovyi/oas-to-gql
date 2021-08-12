package oas_utils

import (
	"bytes"
	"encoding/json"
	"io"
	"openapi-to-graphql/utils"
)

type Body struct {
	ContentType string
	Data        interface{}
}

func (b *Body) Encode() io.Reader {
	switch b.ContentType {
	case "application/json":
		var jsonStr, err = json.Marshal(b.Data)
		if err != nil {
			return &bytes.Buffer{}
		}
		return bytes.NewBuffer([]byte(jsonStr))
	case "application/x-www-form-urlencoded":
		serialized := utils.Serialize(b.Data, "")
		return bytes.NewBuffer([]byte(serialized))
	}

	return &bytes.Buffer{}
}
