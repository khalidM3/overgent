package multilang

// PHP's top-level functions, classes, interfaces, traits and enums are always
// reachable — there is no file-private declaration — so they are surface
// unconditionally. Class members default to public and are excluded only when a
// visibility modifier explicitly says private or protected.

var phpBodies = []string{"declaration_list", "compound_statement", "enum_declaration_list"}

func collectPHP(v view) []declaration {
	if len(v.records) == 0 {
		return nil
	}
	var found []declaration
	for _, top := range v.children(0) {
		found = appendPHPNode(v, found, top, "")
	}
	return found
}

func appendPHPNode(v view, found []declaration, node int, prefix string) []declaration {
	switch v.kind(node) {
	case "namespace_definition":
		body, ok := v.childOfKind(node, "compound_statement", "declaration_list")
		if !ok {
			return found
		}
		for _, member := range v.children(body) {
			found = appendPHPNode(v, found, member, prefix)
		}
		return found
	case "function_definition":
		name, ok := phpName(v, node)
		if !ok {
			return found
		}
		return append(found, declaration{name: prefix + name, kind: kindFunction, raw: v.header(node, phpBodies...)})
	}

	kind, ok := phpTypeKind(v.kind(node))
	if !ok {
		return found
	}
	name, ok := phpName(v, node)
	if !ok {
		return found
	}
	qualified := prefix + name
	found = append(found, declaration{name: qualified, kind: kind, raw: v.header(node, phpBodies...)})

	body, ok := v.childOfKind(node, phpBodies...)
	if !ok {
		return found
	}
	for _, member := range v.children(body) {
		found = appendPHPMember(v, found, member, qualified+"::")
	}
	return found
}

func appendPHPMember(v view, found []declaration, node int, prefix string) []declaration {
	switch v.kind(node) {
	case "method_declaration":
		if phpIsHidden(v, node) {
			return found
		}
		name, ok := phpName(v, node)
		if !ok {
			return found
		}
		return append(found, declaration{name: prefix + name, kind: kindMethod, raw: v.header(node, phpBodies...)})
	case "property_declaration":
		if phpIsHidden(v, node) {
			return found
		}
		// A property's name is a variable_name holding the $identifier.
		target, ok := v.childOfKind(node, "property_element")
		if !ok {
			return found
		}
		variable, ok := v.childOfKind(target, "variable_name")
		if !ok {
			return found
		}
		return append(found, declaration{name: prefix + v.text(variable), kind: kindField, raw: phpPropertyHeader(v, node, target)})
	case "const_declaration":
		if phpIsHidden(v, node) {
			return found
		}
		element, ok := v.childOfKind(node, "const_element")
		if !ok {
			return found
		}
		target, ok := v.childOfKind(element, "name")
		if !ok {
			return found
		}
		return append(found, declaration{name: prefix + v.text(target), kind: kindConst, raw: prefix + v.text(target)})
	case "enum_case":
		name, ok := phpName(v, node)
		if !ok {
			return found
		}
		return append(found, declaration{name: prefix + name, kind: kindField, raw: name})
	}
	return found
}

// phpPropertyHeader keeps the modifiers and type but drops any default value.
func phpPropertyHeader(v view, property, element int) string {
	if equals, ok := v.childOfKind(element, "="); ok {
		return v.textTo(property, equals)
	}
	return v.text(property)
}

// phpIsHidden reports whether a member is explicitly private or protected.
// Absence of a modifier means public, which is PHP's default.
func phpIsHidden(v view, node int) bool {
	modifier, ok := v.childOfKind(node, "visibility_modifier")
	if !ok {
		return false
	}
	switch v.text(modifier) {
	case "private", "protected":
		return true
	}
	return false
}

func phpTypeKind(node string) (string, bool) {
	switch node {
	case "class_declaration":
		return kindClass, true
	case "interface_declaration":
		return kindInterface, true
	case "trait_declaration":
		return kindInterface, true
	case "enum_declaration":
		return kindEnum, true
	}
	return "", false
}

func phpName(v view, node int) (string, bool) {
	identifier, ok := v.childOfKind(node, "name")
	if !ok {
		return "", false
	}
	name := v.text(identifier)
	if name == "" {
		return "", false
	}
	return name, true
}
