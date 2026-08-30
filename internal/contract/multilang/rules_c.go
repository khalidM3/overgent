package multilang

import "strings"

// C has no visibility keyword: `static` gives a definition internal linkage,
// and everything else is reachable from another translation unit. So the rule
// inverts the others — a declaration is surface unless it is marked static.
//
// A header's declarations are the contract other files compile against, and a
// .c file's non-static definitions are what they link against. Both are
// recorded, and both reduce to the same declarator text, so a prototype and its
// definition produce the same signature.

var cBodies = []string{"compound_statement", "field_declaration_list", "enumerator_list"}

func collectC(v view) []declaration {
	if len(v.records) == 0 {
		return nil
	}
	var found []declaration
	for _, top := range v.children(0) {
		found = appendCNode(v, found, top, "")
	}
	return found
}

func appendCNode(v view, found []declaration, node int, prefix string) []declaration {
	switch v.kind(node) {
	case "function_definition", "declaration":
		if cIsStatic(v, node) {
			return found
		}
		// A declaration may introduce a type rather than a value, in which case
		// the type specifier is the surface and there is no declarator.
		if name, kind, ok := cTypeSpecifier(v, node); ok {
			return append(found, declaration{name: prefix + name, kind: kind, raw: v.header(node, cBodies...)})
		}
		declarator, ok := v.childOfKind(node, "function_declarator", "pointer_declarator", "init_declarator", "identifier", "array_declarator")
		if !ok {
			return found
		}
		name, ok := cDeclaratorName(v, declarator)
		if !ok {
			return found
		}
		kind := kindConst
		if cIsFunction(v, declarator) {
			kind = kindFunction
		}
		return append(found, declaration{name: prefix + name, kind: kind, raw: cHeader(v, node, declarator)})
	case "type_definition":
		// typedef names the type at its declarator, not its specifier.
		declarator, ok := v.childOfKind(node, "type_identifier", "pointer_declarator", "function_declarator")
		if !ok {
			return found
		}
		name, ok := cDeclaratorName(v, declarator)
		if !ok {
			return found
		}
		return append(found, declaration{name: prefix + name, kind: kindAlias, raw: v.text(node)})
	case "struct_specifier", "union_specifier", "enum_specifier":
		name, kind, ok := cSpecifierNameKind(v, node)
		if !ok {
			return found
		}
		return append(found, declaration{name: prefix + name, kind: kind, raw: v.header(node, cBodies...)})
	}
	return found
}

// cTypeSpecifier recognizes a declaration whose whole purpose is to introduce a
// struct, union or enum rather than to declare a value.
func cTypeSpecifier(v view, node int) (string, string, bool) {
	specifier, ok := v.childOfKind(node, "struct_specifier", "union_specifier", "enum_specifier")
	if !ok {
		return "", "", false
	}
	// A declaration that also has a declarator is declaring a variable of that
	// type, so the variable is the surface, not the type.
	if _, hasDeclarator := v.childOfKind(node, "function_declarator", "pointer_declarator", "init_declarator", "identifier", "array_declarator"); hasDeclarator {
		return "", "", false
	}
	name, kind, ok := cSpecifierNameKind(v, specifier)
	return name, kind, ok
}

func cSpecifierNameKind(v view, node int) (string, string, bool) {
	identifier, ok := v.childOfKind(node, "type_identifier")
	if !ok {
		return "", "", false
	}
	kind := kindClass
	if v.kind(node) == "enum_specifier" {
		kind = kindEnum
	}
	return v.text(identifier), kind, true
}

// cDeclaratorName unwraps pointer and array declarators to reach the name.
func cDeclaratorName(v view, node int) (string, bool) {
	for depth := 0; depth < 8; depth++ {
		switch v.kind(node) {
		case "identifier", "type_identifier", "field_identifier":
			name := v.text(node)
			return name, name != ""
		}
		inner, ok := v.childOfKind(node, "function_declarator", "pointer_declarator", "array_declarator", "parenthesized_declarator", "identifier", "type_identifier", "field_identifier")
		if !ok {
			return "", false
		}
		node = inner
	}
	return "", false
}

func cIsFunction(v view, declarator int) bool {
	for depth := 0; depth < 8; depth++ {
		if v.kind(declarator) == "function_declarator" {
			return true
		}
		inner, ok := v.childOfKind(declarator, "function_declarator", "pointer_declarator", "array_declarator", "parenthesized_declarator")
		if !ok {
			return false
		}
		declarator = inner
	}
	return false
}

// cHeader drops a function body and drops an initializer, so neither the
// implementation nor a constant's value is part of the contract.
func cHeader(v view, node, declarator int) string {
	if body, ok := v.childOfKind(node, cBodies...); ok {
		return strings.TrimRight(v.textTo(node, body), " \t\r\n")
	}
	if v.kind(declarator) == "init_declarator" {
		if equals, ok := v.childOfKind(declarator, "="); ok {
			return v.textTo(node, equals)
		}
	}
	// A prototype ends in a semicolon and its definition does not. Trimming it
	// makes the two produce one identical signature, so declaring a function in
	// a header and defining it in a .c file is one contract, not two.
	return strings.TrimRight(strings.TrimRight(v.text(node), " \t\r\n"), ";")
}

func cIsStatic(v view, node int) bool {
	for _, child := range v.children(node) {
		if v.kind(child) == "storage_class_specifier" && v.text(child) == "static" {
			return true
		}
	}
	return false
}
