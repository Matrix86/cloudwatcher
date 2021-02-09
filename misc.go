package cloudwatcher

import (
	"fmt"
	"reflect"
	"strings"
)

func convertStructToMap(st interface{}) map[string]string {

	reqRules := make(map[string]string)

	v := reflect.ValueOf(st)
	t := reflect.TypeOf(st)

	if t.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	for i := 0; i < v.NumField(); i++ {
		key := strings.ToLower(t.Field(i).Name)
		typ := v.FieldByName(t.Field(i).Name).Kind().String()
		structTag := t.Field(i).Tag.Get("json")
		jsonName := strings.TrimSpace(strings.Split(structTag, ",")[0])
		value := v.FieldByName(t.Field(i).Name)

		// if jsonName is not empty use it for the key
		if jsonName != ""  && jsonName != "-" {
			key = jsonName
		}

		if typ == "string" {
			reqRules[key] = value.String()
		} else if typ == "int" {
			reqRules[key] = fmt.Sprintf("%d", value.Int())
		} else if typ == "bool" {
			reqRules[key] = fmt.Sprintf("%t", value.Bool())
		} else {
			reqRules[key] = ""
		}

	}

	return reqRules
}
