package multilang

// Java's exported surface is what the `public` modifier marks, plus interface
// members, which are implicitly public. Anything package-private, protected or
// private is not something another workstream can depend on, so it is not
// contract surface and must not raise a stale-assumption finding.

// javaBodies are the nodes that open a declaration body. A header ends where
// one of these begins.
var javaBodies = []string{"class_body", "interface_body", "enum_body", "constructor_body", "block", "annotation_type_body"}

func collectJava(v view) []declaration {
	if len(v.records) == 0 {
		return nil
	}
	var found []declaration
	for _, top := range v.children(0) {
		found = appendJavaType(v, found, top, "")
	}
	return found
}

// appendJavaType records a public type and then walks into its body, because a
// public method on a public class is surface a consumer calls directly.
func appendJavaType(v view, found []declaration, node int, prefix string) []declaration {
	kind, ok := javaTypeKind(v.kind(node))
	if !ok {
		return found
	}
	// A nested type inherits its parent's reachability: it is only surface when
	// the parent was, and the parent is only walked into when it was public.
	if !javaIsPublic(v, node) {
		return found
	}
	name, ok := javaName(v, node)
	if !ok {
		return found
	}
	qualified := prefix + name
	found = append(found, declaration{name: qualified, kind: kind, raw: v.header(node, javaBodies...)})

	body, ok := v.childOfKind(node, javaBodies...)
	if !ok {
		return found
	}
	// An interface's members carry no modifiers and are public by definition.
	implicit := v.kind(node) == "interface_declaration" || v.kind(node) == "annotation_type_declaration"
	for _, member := range v.children(body) {
		found = appendJavaMember(v, found, member, qualified+".", implicit)
	}
	return found
}

func appendJavaMember(v view, found []declaration, node int, prefix string, implicitlyPublic bool) []declaration {
	if _, ok := javaTypeKind(v.kind(node)); ok {
		return appendJavaType(v, found, node, prefix)
	}
	switch v.kind(node) {
	case "method_declaration", "constructor_declaration", "compact_constructor_declaration":
		if !implicitlyPublic && !javaIsPublic(v, node) {
			return found
		}
		name, ok := javaName(v, node)
		if !ok {
			return found
		}
		return append(found, declaration{name: prefix + name, kind: kindMethod, raw: v.header(node, javaBodies...)})
	case "field_declaration", "constant_declaration":
		if !implicitlyPublic && !javaIsPublic(v, node) {
			return found
		}
		// A field's name sits inside its declarator, and the initializer is not
		// contract surface: changing a value is not changing a signature.
		declarator, ok := v.childOfKind(node, "variable_declarator")
		if !ok {
			return found
		}
		target, ok := v.childOfKind(declarator, "identifier")
		if !ok {
			return found
		}
		return append(found, declaration{name: prefix + v.text(target), kind: kindField, raw: javaFieldHeader(v, node, declarator)})
	case "enum_constant":
		name, ok := javaName(v, node)
		if !ok {
			return found
		}
		return append(found, declaration{name: prefix + name, kind: kindField, raw: name})
	}
	return found
}

// javaFieldHeader keeps the modifiers, type and name but drops any initializer.
func javaFieldHeader(v view, field, declarator int) string {
	if assign, ok := v.childOfKind(declarator, "="); ok {
		return v.textTo(field, assign)
	}
	return v.text(field)
}

func javaTypeKind(node string) (string, bool) {
	switch node {
	case "class_declaration", "record_declaration":
		return kindClass, true
	case "interface_declaration", "annotation_type_declaration":
		return kindInterface, true
	case "enum_declaration":
		return kindEnum, true
	}
	return "", false
}

// javaIsPublic reports whether a declaration carries the public modifier.
func javaIsPublic(v view, node int) bool {
	modifiers, ok := v.childOfKind(node, "modifiers")
	if !ok {
		return false
	}
	for _, modifier := range v.children(modifiers) {
		if v.kind(modifier) == "public" {
			return true
		}
	}
	return false
}

func javaName(v view, node int) (string, bool) {
	identifier, ok := v.childOfKind(node, "identifier")
	if !ok {
		return "", false
	}
	name := v.text(identifier)
	if name == "" {
		return "", false
	}
	return name, true
}
