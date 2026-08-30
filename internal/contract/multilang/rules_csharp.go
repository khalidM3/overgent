package multilang

// C#'s exported surface is what `public` marks. Interface members carry no
// modifiers and are public by definition. Namespaces are containers rather
// than surface, so their declaration lists are walked through without being
// recorded themselves.

var csharpBodies = []string{"declaration_list", "enum_member_declaration_list", "block", "accessor_list", "arrow_expression_clause"}

func collectCSharp(v view) []declaration {
	if len(v.records) == 0 {
		return nil
	}
	var found []declaration
	for _, top := range v.children(0) {
		found = appendCSharpNode(v, found, top, "", false)
	}
	return found
}

func appendCSharpNode(v view, found []declaration, node int, prefix string, implicitlyPublic bool) []declaration {
	switch v.kind(node) {
	case "namespace_declaration", "file_scoped_namespace_declaration":
		// A namespace is a path, not a declaration. Walk through it and keep
		// its name as the prefix so members stay distinguishable.
		name, _ := csharpName(v, node)
		inner := prefix
		if name != "" {
			inner = prefix + name + "."
		}
		body, ok := v.childOfKind(node, "declaration_list")
		if !ok {
			// A file-scoped namespace has no body; its siblings follow it.
			return found
		}
		for _, member := range v.children(body) {
			found = appendCSharpNode(v, found, member, inner, false)
		}
		return found
	}

	if kind, ok := csharpTypeKind(v.kind(node)); ok {
		if !implicitlyPublic && !csharpIsPublic(v, node) {
			return found
		}
		name, ok := csharpName(v, node)
		if !ok {
			return found
		}
		qualified := prefix + name
		found = append(found, declaration{name: qualified, kind: kind, raw: v.header(node, csharpBodies...)})
		body, ok := v.childOfKind(node, csharpBodies...)
		if !ok {
			return found
		}
		implicit := v.kind(node) == "interface_declaration"
		for _, member := range v.children(body) {
			found = appendCSharpNode(v, found, member, qualified+".", implicit)
		}
		return found
	}

	switch v.kind(node) {
	case "method_declaration", "constructor_declaration", "operator_declaration", "conversion_operator_declaration", "delegate_declaration":
		if !implicitlyPublic && !csharpIsPublic(v, node) {
			return found
		}
		name, ok := csharpName(v, node)
		if !ok {
			return found
		}
		return append(found, declaration{name: prefix + name, kind: kindMethod, raw: v.header(node, csharpBodies...)})
	case "property_declaration", "event_declaration", "indexer_declaration":
		if !implicitlyPublic && !csharpIsPublic(v, node) {
			return found
		}
		name, ok := csharpName(v, node)
		if !ok {
			return found
		}
		return append(found, declaration{name: prefix + name, kind: kindField, raw: v.header(node, csharpBodies...)})
	case "field_declaration", "event_field_declaration":
		if !implicitlyPublic && !csharpIsPublic(v, node) {
			return found
		}
		declaration_, ok := v.childOfKind(node, "variable_declaration")
		if !ok {
			return found
		}
		declarator, ok := v.childOfKind(declaration_, "variable_declarator")
		if !ok {
			return found
		}
		target, ok := v.childOfKind(declarator, "identifier")
		if !ok {
			return found
		}
		return append(found, declaration{name: prefix + v.text(target), kind: kindField, raw: csharpFieldHeader(v, node, declarator)})
	case "enum_member_declaration":
		name, ok := csharpName(v, node)
		if !ok {
			return found
		}
		return append(found, declaration{name: prefix + name, kind: kindField, raw: name})
	}
	return found
}

// csharpFieldHeader drops an initializer so a changed value is not a changed
// contract.
func csharpFieldHeader(v view, field, declarator int) string {
	if equals, ok := v.childOfKind(declarator, "="); ok {
		return v.textTo(field, equals)
	}
	return v.text(field)
}

func csharpTypeKind(node string) (string, bool) {
	switch node {
	case "class_declaration", "struct_declaration", "record_declaration", "record_struct_declaration":
		return kindClass, true
	case "interface_declaration":
		return kindInterface, true
	case "enum_declaration":
		return kindEnum, true
	}
	return "", false
}

func csharpIsPublic(v view, node int) bool {
	for _, child := range v.children(node) {
		if v.kind(child) != "modifier" {
			continue
		}
		if v.text(child) == "public" {
			return true
		}
	}
	return false
}

func csharpName(v view, node int) (string, bool) {
	identifier, ok := v.childOfKind(node, "identifier", "qualified_name")
	if !ok {
		return "", false
	}
	name := v.text(identifier)
	if name == "" {
		return "", false
	}
	return name, true
}
