package main

import (
	"strings"

	"google.golang.org/protobuf/compiler/protogen"
)

func generatePostgresFile(gen *protogen.Plugin, file *protogen.File) {
	for _, message := range file.Messages {
		filename := message.GoIdent.GoName + ".postgres.orm.go"

		// if we have filter and after checking the filter map, we don't have the message name, skip generating ORM
		if *filter != "" {
			if _, ok := filterMap[message.GoIdent.GoName]; !ok {
				return
			}
		}

		genFile := gen.NewGeneratedFile(filename, file.GoImportPath)
		printHeader(gen, genFile, file)

		genFile.P("import (")
		genFile.P("\"strings\"")
		genFile.P("\"github.com/Masterminds/squirrel\"")
		genFile.P(")")
		genFile.P()

		// Generate the BuildFilter method
		structName := "Filter" + message.GoIdent.GoName

		generateBuildPostgresFilterMethod(genFile, structName, message.Fields)
	}
}

func generateBuildPostgresFilterMethod(genFile *protogen.GeneratedFile, structName string, fields []*protogen.Field) {
	genFile.P("func (f *", structName, ") BuildFilter(query squirrel.SelectBuilder) squirrel.SelectBuilder {")

	for _, field := range fields {
		if field.Desc.IsList() || field.Desc.IsMap() {
			continue
		}

		fieldName := field.GoName
		dbColumnName := strings.ToLower(fieldName)
		generateFieldFilterConditions(genFile, fieldName, dbColumnName)
	}

	genFile.P("return query")
	genFile.P("}")
}

func generateFieldFilterConditions(genFile *protogen.GeneratedFile, fieldName, dbColumnName string) {
	conditions := []string{"Eq", "Ne", "Lt", "Le", "Gt", "Ge", "StartsWith", "EndsWith"}
	operators := []string{"=", "<>", "<", "<=", ">", ">=", "LIKE", "LIKE"}

	genFile.P("if f.", fieldName, " != nil {") // nil pointer check

	for i, cond := range conditions {
		operator := operators[i]

		var valuePlaceholder string

		switch cond {
		case "StartsWith":
			valuePlaceholder = "'%' || ?"
		case "EndsWith":
			valuePlaceholder = "? || '%'"
		default:
			valuePlaceholder = "?"
		}

		genFile.P("if f.", fieldName, ".", cond, " != \"\" {")
		genFile.P("query = query.Where(\"", dbColumnName, " ", operator, " ", valuePlaceholder, "\", f.", fieldName, ".", cond, ")")
		genFile.P("}")
	}

	// Handle Contains
	genFile.P("if len(f.", fieldName, ".Contains) > 0 {")
	genFile.P("containsQueries := []string{}")
	genFile.P("containsArgs := []interface{}{}")
	genFile.P("for _, v := range f.", fieldName, ".Contains {")
	genFile.P("    if v != \"\" {")
	genFile.P("        containsQueries = append(containsQueries, \"", dbColumnName, " LIKE ?\")")
	genFile.P("        containsArgs = append(containsArgs, \"%\" + v + \"%\")")
	genFile.P("    }")
	genFile.P("}")
	genFile.P("if len(containsQueries) > 0 {")
	genFile.P("    query = query.Where(\"(\" + strings.Join(containsQueries, \" OR \") + \")\", containsArgs...)")
	genFile.P("}")
	genFile.P("}")

	// Handle NotContains
	genFile.P("if len(f.", fieldName, ".NotContains) > 0 {")
	genFile.P("notContainsQueries := []string{}")
	genFile.P("notContainsArgs := []interface{}{}")
	genFile.P("for _, v := range f.", fieldName, ".NotContains {")
	genFile.P("    if v != \"\" {")
	genFile.P("        notContainsQueries = append(notContainsQueries, \"", dbColumnName, " NOT LIKE ?\")")
	genFile.P("        notContainsArgs = append(notContainsArgs, \"%\" + v + \"%\")")
	genFile.P("    }")
	genFile.P("}")
	genFile.P("if len(notContainsQueries) > 0 {")
	genFile.P("    query = query.Where(\"(\" + strings.Join(notContainsQueries, \" OR \") + \")\", notContainsArgs...)")
	genFile.P("}")
	genFile.P("}")

	genFile.P("if f.", fieldName, ".IsEmpty {")
	genFile.P("query = query.Where(\"", dbColumnName, " = '' OR ", dbColumnName, " IS NULL\")")
	genFile.P("}")

	genFile.P("if f.", fieldName, ".IsNotEmpty {")
	genFile.P("query = query.Where(\"", dbColumnName, " <> '' AND ", dbColumnName, " IS NOT NULL\")")
	genFile.P("}")

	genFile.P("}") // End of nil pointer check
}
