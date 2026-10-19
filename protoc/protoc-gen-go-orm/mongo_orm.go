package main

import (
	"strings"

	"google.golang.org/protobuf/compiler/protogen"
)

func generateMongoFile(gen *protogen.Plugin, file *protogen.File) {
	for _, message := range file.Messages {
		filename := message.GoIdent.GoName + ".mongo.orm.go"

		// if we have filter and after checking the filter map, we don't have the message name, skip generating ORM
		if *filter != "" {
			if _, ok := filterMap[message.GoIdent.GoName]; !ok {
				return
			}
		}

		genFile := gen.NewGeneratedFile(filename, file.GoImportPath)
		printHeader(gen, genFile, file)

		genFile.P("import (")
		genFile.P("\"go.mongodb.org/mongo-driver/v2/bson\"")
		genFile.P(")")
		genFile.P()

		// Generate the BuildFilter method
		structName := "Filter" + message.GoIdent.GoName

		generateBuildMongoFilterMethod(genFile, structName, message.Fields)
	}
}

func generateBuildMongoFilterMethod(genFile *protogen.GeneratedFile, structName string, fields []*protogen.Field) {
	genFile.P("func (f *", structName, ") BuildMongoFilter() bson.M {")
	genFile.P("if f == nil {")
	genFile.P("return nil")
	genFile.P("}")
	genFile.P("filter := bson.M{}")

	for _, field := range fields {
		if field.Desc.IsList() || field.Desc.IsMap() {
			continue
		}

		fieldName := field.GoName
		bsonFieldName := strings.ToLower(fieldName) // Adjust as per your field naming conventions
		genFile.P("if f.", fieldName, " != nil {")
		genFile.P("fieldFilter := bson.M{}")

		// Logic for each condition in the StringFilterInput struct
		genFile.P("if f.", fieldName, ".Eq != \"\" {")
		genFile.P("fieldFilter[\"$eq\"] = f.", fieldName, ".Eq")
		genFile.P("}")
		genFile.P("if f.", fieldName, ".Ne != \"\" {")
		genFile.P("fieldFilter[\"$ne\"] = f.", fieldName, ".Ne")
		genFile.P("}")
		genFile.P("if f.", fieldName, ".Lt != \"\" {")
		genFile.P("fieldFilter[\"$lt\"] = f.", fieldName, ".Lt")
		genFile.P("}")
		genFile.P("if f.", fieldName, ".Le != \"\" {")
		genFile.P("fieldFilter[\"$lte\"] = f.", fieldName, ".Le")
		genFile.P("}")
		genFile.P("if f.", fieldName, ".Gt != \"\" {")
		genFile.P("fieldFilter[\"$gt\"] = f.", fieldName, ".Gt")
		genFile.P("}")
		genFile.P("if f.", fieldName, ".Ge != \"\" {")
		genFile.P("fieldFilter[\"$gte\"] = f.", fieldName, ".Ge")
		genFile.P("}")

		// Handle Contains as an array
		genFile.P("if len(f.", fieldName, ".Contains) > 0 {")
		genFile.P("fieldFilter[\"$in\"] = f.", fieldName, ".Contains")
		genFile.P("}")

		// Handle NotContains as an array
		genFile.P("if len(f.", fieldName, ".NotContains) > 0 {")
		genFile.P("fieldFilter[\"$nin\"] = f.", fieldName, ".NotContains")
		genFile.P("}")

		genFile.P("if len(fieldFilter) > 0 {")
		genFile.P("filter[\"", bsonFieldName, "\"] = fieldFilter")
		genFile.P("}")
		genFile.P("}")
	}

	genFile.P("return filter")
	genFile.P("}")
}
