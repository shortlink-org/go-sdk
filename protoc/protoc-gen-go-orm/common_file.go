package main

import (
	"strings"

	"google.golang.org/protobuf/compiler/protogen"
)

func generateCommonFile(gen *protogen.Plugin, file *protogen.File) {
	genFile := gen.NewGeneratedFile(commonFilename, file.GoImportPath)
	printHeader(gen, genFile, file)

	// Generate the StringFilterInput struct definition
	genFile.P("type StringFilterInput struct { //nolint:unused")
	genFile.P("    Eq           string    `json:\"eq,omitempty\"`")          // Equal
	genFile.P("    Ne           string    `json:\"ne,omitempty\"`")          // Not Equal
	genFile.P("    Lt           string    `json:\"lt,omitempty\"`")          // Less Than
	genFile.P("    Le           string    `json:\"le,omitempty\"`")          // Less Than or Equal
	genFile.P("    Gt           string    `json:\"gt,omitempty\"`")          // Greater Than
	genFile.P("    Ge           string    `json:\"ge,omitempty\"`")          // Greater Than or Equal
	genFile.P("    Contains     []string  `json:\"contains,omitempty\"`")    // Contains
	genFile.P("    NotContains  []string  `json:\"notContains,omitempty\"`") // Not Contains
	genFile.P("    StartsWith   string    `json:\"startsWith,omitempty\"`")  // Starts With
	genFile.P("    EndsWith     string    `json:\"endsWith,omitempty\"`")    // Ends With
	genFile.P("    IsEmpty      bool      `json:\"isEmpty,omitempty\"`")     // Is Empty
	genFile.P("    IsNotEmpty   bool      `json:\"isNotEmpty,omitempty\"`")  // Is Not Empty
	genFile.P("}")
	genFile.P()

	for _, message := range file.Messages {
		if _, ok := filterMap[message.GoIdent.GoName]; !ok && len(filterMap) > 0 {
			continue
		}

		// Generate the BuildFilter method
		structName := "Filter" + message.GoIdent.GoName
		genFile.P("type ", structName, " struct {")

		for _, field := range message.Fields {
			if field.Desc.IsList() || field.Desc.IsMap() {
				continue
			}

			fieldName := field.GoName
			goType := "*" + "StringFilterInput"
			genFile.P(fieldName, " ", goType, " `json:\"", strings.ToLower(fieldName), "\"`")
		}

		genFile.P("}")
		genFile.P()
	}
}

func useCommonFile(gen *protogen.Plugin, file *protogen.File) {
	genFile := gen.NewGeneratedFile(commonFilename, file.GoImportPath)
	printHeader(gen, genFile, file)

	genFile.P("import (")
	genFile.P("    domain \"", *commonPath, "\"")
	genFile.P(")")

	for _, message := range file.Messages {
		if _, ok := filterMap[message.GoIdent.GoName]; !ok && len(filterMap) > 0 {
			continue
		}

		// Generate the BuildFilter method
		structName := "Filter" + message.GoIdent.GoName

		// use commonPath as alias
		genFile.P("type ", structName, " domain.", structName)

		// Construct the NewFilter function
		genFile.P("func NewFilter(params *domain.", structName, ") *", structName, " {")

		// check if the filter is nil
		genFile.P("    if params == nil {")
		genFile.P("        return nil")
		genFile.P("    }")
		genFile.P()

		genFile.P("    return &", structName, "{")

		for _, message := range file.Messages {
			if _, ok := filterMap[message.GoIdent.GoName]; !ok && len(filterMap) > 0 {
				continue
			}

			for _, field := range message.Fields {
				if field.Desc.IsList() || field.Desc.IsMap() {
					continue
				}

				fieldName := field.GoName
				genFile.P(fieldName, ": params.", fieldName, ",")
			}
		}

		genFile.P("    }")
		genFile.P("}")
		genFile.P()
	}
}
