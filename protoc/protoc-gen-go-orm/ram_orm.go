package main

import (
	"strconv"

	"google.golang.org/protobuf/compiler/protogen"
)

func generateRAMFile(gen *protogen.Plugin, file *protogen.File) {
	for _, message := range file.Messages {
		filename := message.GoIdent.GoName + ".ram.orm.go"

		// if we have filter and after checking the filter map, we don't have the message name, skip generating ORM
		if *filter != "" {
			if _, ok := filterMap[message.GoIdent.GoName]; !ok {
				return
			}
		}

		genFile := gen.NewGeneratedFile(filename, file.GoImportPath)
		printHeader(gen, genFile, file)

		genFile.P("import (")
		genFile.P("\"reflect\"")
		genFile.P("\"strings\"")
		genFile.P()

		if !*commonEnable {
			genFile.P("    domain \"", *commonPath, "\"")
		}

		genFile.P(")")
		genFile.P()

		// Generate the BuildFilter method
		structName := "Filter" + message.GoIdent.GoName

		generateBuildRAMFilterMethod(genFile, structName, message.Fields)
	}
}

func generateBuildRAMFilterMethod(genFile *protogen.GeneratedFile, structName string, fields []*protogen.Field) {
	genFile.P("func (f *", structName, ") BuildRAMFilter(item any) bool {")

	if *commonEnable {
		genFile.P("var fieldVal *StringFilterInput")
	} else {
		genFile.P("var fieldVal *domain.StringFilterInput")
	}

	genFile.P("var ok bool")
	genFile.P("var v reflect.Value")
	genFile.P()

	for _, field := range fields {
		if field.Desc.IsList() || field.Desc.IsMap() {
			continue
		}

		fieldName := field.GoName
		generateBuildRAMFieldFilterConditions(genFile, fieldName)
	}

	genFile.P("return true")
	genFile.P("}")
}

func generateBuildRAMFieldFilterConditions(genFile *protogen.GeneratedFile, fieldName string) {
	var stringFilterInput string
	if *commonEnable {
		stringFilterInput = "StringFilterInput"
	} else {
		stringFilterInput = "domain.StringFilterInput"
	}

	genFile.P("fieldVal, ok = reflect.ValueOf(f).Elem().FieldByName(", strconv.Quote(fieldName), ").Interface().(*", stringFilterInput, ")")
	genFile.P("if !ok || fieldVal == nil { return true } // If field is not found or nil, no filtering is applied")
	genFile.P("v = reflect.ValueOf(item).Elem().FieldByName(", strconv.Quote(fieldName), ")")
	genFile.P("if !v.IsValid() { return false } // If the link does not have this field, fail the filter")

	// Generate condition checks
	genFile.P("if fieldVal.Eq != \"\" && v.String() != fieldVal.Eq { return false }")
	genFile.P("if fieldVal.Ne != \"\" && v.String() == fieldVal.Ne { return false }")
	genFile.P("if fieldVal.Lt != \"\" && !(v.String() < fieldVal.Lt) { return false }")
	genFile.P("if fieldVal.Le != \"\" && !(v.String() <= fieldVal.Le) { return false }")
	genFile.P("if fieldVal.Gt != \"\" && !(v.String() > fieldVal.Gt) { return false }")
	genFile.P("if fieldVal.Ge != \"\" && !(v.String() >= fieldVal.Ge) { return false }")
	genFile.P("if fieldVal.StartsWith != \"\" && !strings.HasPrefix(v.String(), fieldVal.StartsWith) { return false }")
	genFile.P("if fieldVal.EndsWith != \"\" && !strings.HasSuffix(v.String(), fieldVal.EndsWith) { return false }")

	// Handle Contains and NotContains for slice of strings
	genFile.P("for _, contain := range fieldVal.Contains {")
	genFile.P("    if !strings.Contains(v.String(), contain) { return false }")
	genFile.P("}")
	genFile.P("for _, notContain := range fieldVal.NotContains {")
	genFile.P("    if strings.Contains(v.String(), notContain) { return false }")
	genFile.P("}")
	genFile.P()

	// Handle IsEmpty and IsNotEmpty
	genFile.P("if fieldVal.IsEmpty && v.String() != \"\" { return false }")
	genFile.P("if fieldVal.IsNotEmpty && v.String() == \"\" { return false }")
}
