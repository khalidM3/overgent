package contract

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"strings"
)

// Go symbol kinds. A method, struct field, or interface member is named
// "Type.Member" so every symbol in a file has a stable, unique identity.
const (
	kindFunc            = "func"
	kindMethod          = "method"
	kindType            = "type"
	kindField           = "field"
	kindInterfaceMember = "interface_member"
	kindConst           = "const"
	kindVar             = "var"
)

// extractGo records the exported Go surface of one file. Comments are not
// parsed at all, so a signature can never carry doc text. A syntax error yields
// no fingerprint rather than a partial one.
func extractGo(source []byte) ([]Symbol, bool) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "source.go", source, parser.SkipObjectResolution)
	if err != nil {
		return nil, false
	}
	var symbols []Symbol
	for _, declaration := range file.Decls {
		switch typed := declaration.(type) {
		case *ast.FuncDecl:
			if symbol, ok := goFunc(fset, typed); ok {
				symbols = append(symbols, symbol)
			}
		case *ast.GenDecl:
			symbols = append(symbols, goGenDecl(fset, typed)...)
		}
	}
	return symbols, true
}

func goFunc(fset *token.FileSet, declaration *ast.FuncDecl) (Symbol, bool) {
	if declaration.Name == nil || !declaration.Name.IsExported() {
		return Symbol{}, false
	}
	name, kind := declaration.Name.Name, kindFunc
	if declaration.Recv != nil {
		receiver, ok := receiverTypeName(declaration.Recv)
		if !ok || !ast.IsExported(receiver) {
			return Symbol{}, false
		}
		name, kind = receiver+"."+declaration.Name.Name, kindMethod
	}
	// The body is dropped before printing, so only the header is ever derived.
	header := *declaration
	header.Body = nil
	header.Doc = nil
	return newSymbol(name, kind, printNode(fset, &header)), true
}

func goGenDecl(fset *token.FileSet, declaration *ast.GenDecl) []Symbol {
	var symbols []Symbol
	for _, spec := range declaration.Specs {
		switch typed := spec.(type) {
		case *ast.TypeSpec:
			symbols = append(symbols, goTypeSpec(fset, typed)...)
		case *ast.ValueSpec:
			keyword := kindConst
			if declaration.Tok == token.VAR {
				keyword = kindVar
			} else if declaration.Tok != token.CONST {
				continue
			}
			symbols = append(symbols, goValueSpec(fset, typed, keyword)...)
		}
	}
	return symbols
}

func goTypeSpec(fset *token.FileSet, spec *ast.TypeSpec) []Symbol {
	if spec.Name == nil || !spec.Name.IsExported() {
		return nil
	}
	name := spec.Name.Name
	// A struct or interface body is described by its own member symbols, so the
	// type signature keeps only the header. Editing one field then reports as
	// that field changing rather than as the whole type changing.
	var header string
	switch underlying := spec.Type.(type) {
	case *ast.StructType:
		header = "type " + name + goTypeParams(fset, spec) + " struct"
	case *ast.InterfaceType:
		header = "type " + name + goTypeParams(fset, spec) + " interface"
	default:
		assign := " "
		if spec.Assign.IsValid() {
			assign = " = "
		}
		header = "type " + name + goTypeParams(fset, spec) + assign + printNode(fset, underlying)
	}
	symbols := []Symbol{newSymbol(name, kindType, header)}
	switch underlying := spec.Type.(type) {
	case *ast.StructType:
		symbols = append(symbols, goFields(fset, name, underlying.Fields, kindField)...)
	case *ast.InterfaceType:
		symbols = append(symbols, goFields(fset, name, underlying.Methods, kindInterfaceMember)...)
	}
	return symbols
}

// goTypeParams renders a generic parameter list. The printer emits a bare
// field list without its brackets, so the brackets are added here.
func goTypeParams(fset *token.FileSet, spec *ast.TypeSpec) string {
	if spec.TypeParams == nil || len(spec.TypeParams.List) == 0 {
		return ""
	}
	parameters := make([]string, 0, len(spec.TypeParams.List))
	for _, field := range spec.TypeParams.List {
		names := make([]string, 0, len(field.Names))
		for _, name := range field.Names {
			names = append(names, name.Name)
		}
		parameters = append(parameters, strings.TrimSpace(strings.Join(names, ", ")+" "+printNode(fset, field.Type)))
	}
	return "[" + strings.Join(parameters, ", ") + "]"
}

// goFields records the exported members of a struct or interface. An embedded
// member has no name of its own and is identified by its printed type, which
// keeps embedding visible in the contract.
func goFields(fset *token.FileSet, owner string, fields *ast.FieldList, kind string) []Symbol {
	if fields == nil {
		return nil
	}
	var symbols []Symbol
	for _, field := range fields.List {
		if len(field.Names) == 0 {
			embedded := normalizeSignature(printNode(fset, field.Type))
			if embedded == "" || !exportedEmbedded(embedded) {
				continue
			}
			symbols = append(symbols, newSymbol(owner+"."+embedded, kind, embedded))
			continue
		}
		for _, name := range field.Names {
			if !name.IsExported() {
				continue
			}
			member := name.Name + " " + printNode(fset, field.Type)
			// An interface method reads as "Method(args) result", not as the
			// printer's anonymous "func(args) result".
			if function, ok := field.Type.(*ast.FuncType); ok && kind == kindInterfaceMember {
				member = name.Name + strings.TrimPrefix(printNode(fset, function), "func")
			}
			if field.Tag != nil {
				member += " " + printNode(fset, field.Tag)
			}
			symbols = append(symbols, newSymbol(owner+"."+name.Name, kind, member))
		}
	}
	return symbols
}

// goValueSpec records exported constants and variables. The declared value is
// part of an exported constant's contract, so it is kept when the spec names it
// positionally; an implicit iota continuation simply has no value text.
func goValueSpec(fset *token.FileSet, spec *ast.ValueSpec, keyword string) []Symbol {
	var symbols []Symbol
	for index, name := range spec.Names {
		if !name.IsExported() {
			continue
		}
		parts := []string{keyword, name.Name}
		if spec.Type != nil {
			parts = append(parts, printNode(fset, spec.Type))
		}
		if index < len(spec.Values) {
			parts = append(parts, "=", printNode(fset, spec.Values[index]))
		}
		symbols = append(symbols, newSymbol(name.Name, keyword, strings.Join(parts, " ")))
	}
	return symbols
}

func receiverTypeName(receiver *ast.FieldList) (string, bool) {
	if receiver == nil || len(receiver.List) != 1 {
		return "", false
	}
	expression := receiver.List[0].Type
	if star, ok := expression.(*ast.StarExpr); ok {
		expression = star.X
	}
	if index, ok := expression.(*ast.IndexExpr); ok {
		expression = index.X
	}
	if index, ok := expression.(*ast.IndexListExpr); ok {
		expression = index.X
	}
	identifier, ok := expression.(*ast.Ident)
	if !ok {
		return "", false
	}
	return identifier.Name, true
}

// exportedEmbedded keeps embedded members whose own name is exported. A
// qualified type such as io.Reader is exported by its selector.
func exportedEmbedded(text string) bool {
	name := text
	name = strings.TrimPrefix(name, "*")
	if index := strings.LastIndex(name, "."); index >= 0 {
		name = name[index+1:]
	}
	if index := strings.IndexAny(name, "[("); index >= 0 {
		name = name[:index]
	}
	return name != "" && ast.IsExported(name)
}

func printNode(fset *token.FileSet, node ast.Node) string {
	var buffer bytes.Buffer
	if err := (&printer.Config{Mode: printer.RawFormat}).Fprint(&buffer, fset, node); err != nil {
		return ""
	}
	return buffer.String()
}
