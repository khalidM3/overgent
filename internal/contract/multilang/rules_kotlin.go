package multilang

import "strings"

// Kotlin declarations are public unless a visibility modifier says otherwise.
// `internal` is kept: it is visible across the whole module, which in a single
// repository is exactly the surface another session depends on. Only `private`
// and `protected` are excluded.

var kotlinBodies = []string{"class_body", "function_body", "enum_class_body"}

func collectKotlin(v view) []declaration {
	if len(v.records) == 0 {
		return nil
	}
	var found []declaration
	for _, top := range v.children(0) {
		found = appendKotlinNode(v, found, top, "")
	}
	return found
}

func appendKotlinNode(v view, found []declaration, node int, prefix string) []declaration {
	kind, ok := kotlinKind(v.kind(node))
	if !ok {
		return found
	}
	if kotlinIsHidden(v, node) {
		return found
	}
	identifier, ok := v.childOfKind(node, "simple_identifier", "type_identifier")
	if !ok {
		// A property names itself one level down, inside its variable
		// declaration, rather than directly.
		binding, bound := v.childOfKind(node, "variable_declaration")
		if !bound {
			return found
		}
		if identifier, ok = v.childOfKind(binding, "simple_identifier"); !ok {
			return found
		}
	}
	name := v.text(identifier)
	if name == "" {
		return found
	}
	qualified := prefix + name
	found = append(found, declaration{name: qualified, kind: kind, raw: kotlinHeader(v, node)})

	body, ok := v.childOfKind(node, "class_body", "enum_class_body")
	if !ok {
		return found
	}
	for _, member := range v.children(body) {
		found = appendKotlinNode(v, found, member, qualified+".")
	}
	return found
}

// kotlinHeader drops a body or an expression-bodied definition's right side.
func kotlinHeader(v view, node int) string {
	if body, ok := v.childOfKind(node, kotlinBodies...); ok {
		return strings.TrimRight(v.textTo(node, body), " \t\r\n")
	}
	if equals, ok := v.childOfKind(node, "="); ok {
		return strings.TrimRight(v.textTo(node, equals), " \t\r\n")
	}
	return v.text(node)
}

func kotlinKind(node string) (string, bool) {
	switch node {
	case "class_declaration":
		return kindClass, true
	case "object_declaration":
		return kindNamespace, true
	case "function_declaration":
		return kindFunction, true
	case "property_declaration":
		return kindConst, true
	case "type_alias":
		return kindAlias, true
	}
	return "", false
}

func kotlinIsHidden(v view, node int) bool {
	modifiers, ok := v.childOfKind(node, "modifiers")
	if !ok {
		return false
	}
	text := v.text(modifiers)
	return strings.Contains(text, "private") || strings.Contains(text, "protected")
}
