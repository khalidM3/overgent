package multilang

import "strings"

// rules is the per-language interpretation of a parse tree. Everything
// language-specific lives here; the wasm guest emits only numeric node ids, so
// adding a language is this file plus a grammar in the wasm build.
type rules struct{ collect func(view) []declaration }

var languageRules = map[string]*rules{
	"python":     {collect: collectPython},
	"javascript": {collect: collectJavaScript},
	// TypeScript and TSX reuse the JavaScript rules and add the declaration
	// forms that only exist in TypeScript. The grammars are separate because
	// tree-sitter ships them separately, not because the rules differ.
	"typescript": {collect: collectJavaScript},
	"tsx":        {collect: collectJavaScript},
}

// exportedPython applies Python's convention: a leading underscore marks a
// name private. Dunder names are not API surface either.
func exportedPython(name string) bool {
	return name != "" && !strings.HasPrefix(name, "_")
}

// typeScriptKind maps a TypeScript-only declaration node to a symbol kind.
func typeScriptKind(node string) string {
	switch node {
	case "interface_declaration":
		return kindInterface
	case "type_alias_declaration":
		return kindAlias
	case "enum_declaration":
		return kindEnum
	case "abstract_class_declaration":
		return kindClass
	}
	return kindNamespace
}

const (
	kindFunction  = "function"
	kindInterface = "interface"
	kindAlias     = "type"
	kindEnum      = "enum"
	kindNamespace = "namespace"
	kindClass     = "class"
	kindMethod    = "method"
	kindConst     = "const"
	kindReexport  = "reexport"
)

func collectPython(v view) []declaration {
	if len(v.records) == 0 {
		return nil
	}
	var found []declaration
	for _, top := range v.children(0) {
		node := top
		// A decorated definition keeps the decorators in the signature: an
		// @property or @staticmethod change is a real contract change.
		if v.kind(node) == "decorated_definition" {
			inner, ok := v.childOfKind(node, "function_definition", "class_definition")
			if !ok {
				continue
			}
			found = appendPython(v, found, node, inner, "")
			continue
		}
		found = appendPython(v, found, node, node, "")
	}
	return found
}

func appendPython(v view, found []declaration, outer, inner int, prefix string) []declaration {
	switch v.kind(inner) {
	case "function_definition":
		name, ok := pythonName(v, inner)
		if !ok {
			return found
		}
		kind := kindFunction
		if prefix != "" {
			kind = kindMethod
		}
		return append(found, declaration{name: prefix + name, kind: kind, raw: v.header(outer, "block")})
	case "class_definition":
		name, ok := pythonName(v, inner)
		if !ok {
			return found
		}
		found = append(found, declaration{name: name, kind: kindClass, raw: v.header(outer, "block")})
		body, ok := v.childOfKind(inner, "block")
		if !ok {
			return found
		}
		for _, member := range v.children(body) {
			node := member
			if v.kind(node) == "decorated_definition" {
				definition, ok := v.childOfKind(node, "function_definition")
				if !ok {
					continue
				}
				found = appendPython(v, found, node, definition, name+".")
				continue
			}
			found = appendPython(v, found, node, node, name+".")
		}
		return found
	case "expression_statement":
		// A module-level assignment is API surface, but its value is not: the
		// same rule the TypeScript scanner applies to an exported const.
		assignment, ok := v.childOfKind(inner, "assignment")
		if !ok {
			return found
		}
		target, ok := v.childOfKind(assignment, "identifier")
		if !ok {
			return found
		}
		name := v.text(target)
		if !exportedPython(name) || prefix != "" {
			return found
		}
		return append(found, declaration{name: name, kind: kindConst, raw: pythonAssignmentHeader(v, assignment)})
	}
	return found
}

// pythonAssignmentHeader keeps the target and any type annotation and drops the
// assigned value.
func pythonAssignmentHeader(v view, assignment int) string {
	children := v.children(assignment)
	for _, child := range children {
		if v.kind(child) == "=" {
			return v.textTo(assignment, child)
		}
	}
	return v.text(assignment)
}

func pythonName(v view, index int) (string, bool) {
	identifier, ok := v.childOfKind(index, "identifier")
	if !ok {
		return "", false
	}
	name := v.text(identifier)
	if !exportedPython(name) {
		return "", false
	}
	return name, true
}

// JavaScript node types that open a body and therefore end a header capture.
var jsBodies = []string{"statement_block", "class_body"}

func collectJavaScript(v view) []declaration {
	if len(v.records) == 0 {
		return nil
	}
	var found []declaration
	for _, top := range v.children(0) {
		switch v.kind(top) {
		case "export_statement":
			found = append(found, javaScriptExport(v, top)...)
		case "expression_statement":
			// Most real-world .js is still CommonJS. A token scanner cannot
			// tell `exports.x = f` from an ordinary assignment without
			// tracking scope; with a parse tree it is one node shape.
			found = append(found, javaScriptCommonJS(v, top)...)
		}
	}
	return found
}

// javaScriptCommonJS records `exports.name = …`, `module.exports.name = …` and
// `module.exports = { … }`. As with an exported const, the assigned value is
// not part of the contract unless it is a function, whose header is.
func javaScriptCommonJS(v view, statement int) []declaration {
	assignment, ok := v.childOfKind(statement, "assignment_expression")
	if !ok {
		return nil
	}
	children := v.children(assignment)
	if len(children) < 2 {
		return nil
	}
	left, right := children[0], children[len(children)-1]
	target := v.text(left)
	if target == "module.exports" || target == "exports" {
		return javaScriptExportsObject(v, right)
	}
	if !strings.HasPrefix(target, "exports.") && !strings.HasPrefix(target, "module.exports.") {
		return nil
	}
	name := target[strings.LastIndexByte(target, '.')+1:]
	kind, suffix := kindConst, ""
	switch v.kind(right) {
	case "function_expression", "arrow_function", "generator_function":
		kind = kindFunction
		suffix = " = " + strings.TrimSpace(v.header(right, jsBodies...))
	}
	return []declaration{{name: name, kind: kind, raw: target + suffix}}
}

func javaScriptExportsObject(v view, object int) []declaration {
	if v.kind(object) != "object" {
		return nil
	}
	var found []declaration
	for _, property := range v.children(object) {
		switch v.kind(property) {
		case "pair":
			key, ok := v.childOfKind(property, "property_identifier", "string")
			if !ok {
				continue
			}
			found = append(found, declaration{name: strings.Trim(v.text(key), `"'`), kind: kindReexport, raw: "module.exports." + v.text(key)})
		case "shorthand_property_identifier":
			found = append(found, declaration{name: v.text(property), kind: kindReexport, raw: "module.exports." + v.text(property)})
		}
	}
	return found
}

func javaScriptExport(v view, statement int) []declaration {
	var found []declaration
	for _, child := range v.children(statement) {
		switch v.kind(child) {
		case "function_declaration", "generator_function_declaration":
			if name, ok := javaScriptName(v, child); ok {
				found = append(found, declaration{name: name, kind: kindFunction, raw: v.header(child, jsBodies...)})
			}
		case "class_declaration":
			name, ok := javaScriptName(v, child)
			if !ok {
				continue
			}
			found = append(found, declaration{name: name, kind: kindClass, raw: v.header(child, jsBodies...)})
			body, ok := v.childOfKind(child, "class_body")
			if !ok {
				continue
			}
			for _, member := range v.children(body) {
				if v.kind(member) != "method_definition" {
					continue
				}
				method, ok := v.childOfKind(member, "property_identifier")
				if !ok {
					continue
				}
				found = append(found, declaration{
					name: name + "." + v.text(method),
					kind: kindMethod,
					raw:  v.header(member, jsBodies...),
				})
			}
		case "lexical_declaration", "variable_declaration":
			for _, declarator := range v.children(child) {
				if v.kind(declarator) != "variable_declarator" {
					continue
				}
				name, ok := v.childOfKind(declarator, "identifier")
				if !ok {
					continue
				}
				found = append(found, declaration{
					name: v.text(name),
					kind: kindConst,
					// The initializer is excluded, matching the TypeScript
					// scanner: changing only a literal is not a contract change.
					raw: strings.TrimSpace(v.textTo(child, declarator)) + " " + v.text(name),
				})
			}
		case "interface_declaration", "type_alias_declaration", "enum_declaration", "abstract_class_declaration", "module", "internal_module":
			name, ok := v.childOfKind(child, "type_identifier", "identifier")
			if !ok {
				continue
			}
			found = append(found, declaration{
				name: v.text(name),
				kind: typeScriptKind(v.kind(child)),
				raw:  v.header(child, "interface_body", "class_body", "enum_body", "statement_block"),
			})
		case "function_signature":
			if name, ok := javaScriptName(v, child); ok {
				found = append(found, declaration{name: name, kind: kindFunction, raw: v.text(child)})
			}
		case "export_clause":
			// `export { a, b as c }` — a blind spot of the current TypeScript
			// scanner, and ordinary surface here.
			for _, specifier := range v.children(child) {
				if v.kind(specifier) != "export_specifier" {
					continue
				}
				names := v.children(specifier)
				if len(names) == 0 {
					continue
				}
				exported := v.text(names[len(names)-1])
				found = append(found, declaration{name: exported, kind: kindReexport, raw: v.text(specifier)})
			}
		}
	}
	if len(found) > 0 {
		return found
	}
	// `export default …` and `export * from …` still name a surface even when
	// no declaration node was matched above.
	text := v.text(statement)
	switch {
	case strings.HasPrefix(text, "export default"):
		return []declaration{{name: "default", kind: kindReexport, raw: firstLine(text)}}
	case strings.HasPrefix(text, "export *"):
		return []declaration{{name: "*", kind: kindReexport, raw: firstLine(text)}}
	}
	return nil
}

// javaScriptName accepts type_identifier as well as identifier: the
// TypeScript grammar names a class with the former and the JavaScript grammar
// with the latter.
func javaScriptName(v view, index int) (string, bool) {
	identifier, ok := v.childOfKind(index, "identifier", "type_identifier")
	if !ok {
		return "", false
	}
	return v.text(identifier), true
}

func firstLine(text string) string {
	if index := strings.IndexByte(text, '\n'); index >= 0 {
		return text[:index]
	}
	return text
}
