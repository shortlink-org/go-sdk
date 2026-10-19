package main

import (
	"google.golang.org/protobuf/compiler/protogen"
)

type importManager struct {
	imports map[string]bool
}

func newImportManager() *importManager {
	return &importManager{imports: make(map[string]bool)}
}

func (im *importManager) addImports(imports map[string]bool) {
	for imp := range imports {
		im.imports[imp] = true
	}
}

func (im *importManager) writeImports(genFile *protogen.GeneratedFile) {
	if len(im.imports) > 0 {
		genFile.P("import (")

		for imp := range im.imports {
			genFile.P("\t\"", imp, "\"")
		}

		genFile.P(")")
	}
}
