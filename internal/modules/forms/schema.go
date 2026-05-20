package forms

import (
	"encoding/json"
	"fmt"
	"strings"
)

type FormField struct {
	Key      string   `json:"key"`
	Label    string   `json:"label"`
	Type     string   `json:"type"` // text, textarea, email, checkbox, select
	Required bool     `json:"required"`
	Options  []string `json:"options,omitempty"`
	Help     string   `json:"help,omitempty"`
}

type FormSchema struct {
	Fields []FormField `json:"fields"`
}

func ParseSchema(raw string) (FormSchema, error) {
	var schema FormSchema
	if err := json.Unmarshal([]byte(raw), &schema); err != nil {
		return FormSchema{}, err
	}
	return schema, nil
}

func fieldValueString(val interface{}) string {
	if val == nil {
		return ""
	}
	switch v := val.(type) {
	case string:
		return strings.TrimSpace(v)
	case bool:
		if v {
			return "yes"
		}
		return ""
	default:
		return strings.TrimSpace(fmt.Sprint(v))
	}
}

func fieldValueChecked(val interface{}) bool {
	switch v := val.(type) {
	case bool:
		return v
	case string:
		s := strings.ToLower(strings.TrimSpace(v))
		return s == "true" || s == "yes" || s == "on" || s == "1"
	default:
		return false
	}
}

func (s FormSchema) ValidateSubmission(data map[string]interface{}) error {
	for _, field := range s.Fields {
		val, ok := data[field.Key]
		if !field.Required {
			continue
		}
		if !ok {
			return errRequired(field.Label)
		}
		if field.Type == "checkbox" {
			if !fieldValueChecked(val) {
				return errRequired(field.Label)
			}
			continue
		}
		if fieldValueString(val) == "" {
			return errRequired(field.Label)
		}
	}
	return nil
}

type fieldError string

func (e fieldError) Error() string { return string(e) }

func errRequired(label string) error {
	return fieldError("required field: " + label)
}
