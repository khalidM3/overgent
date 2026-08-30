package multilang

// C++ is C's rules plus namespaces and class access sections. Access is
// positional rather than per-member: a `public:` label switches the mode for
// everything after it, and the starting mode differs between `class` (private)
// and `struct` (public). Getting that backwards would record a class's private
// members as contract surface, so the default is derived from the keyword
// rather than assumed.

func collectCPP(v view) []declaration {
	if len(v.records) == 0 {
		return nil
	}
	var found []declaration
	for _, top := range v.children(0) {
		found = appendCPPNode(v, found, top, "")
	}
	return found
}

func appendCPPNode(v view, found []declaration, node int, prefix string) []declaration {
	switch v.kind(node) {
	case "namespace_definition":
		// A namespace is a path, not surface of its own.
		inner := prefix
		if identifier, ok := v.childOfKind(node, "namespace_identifier", "identifier"); ok {
			inner = prefix + v.text(identifier) + "::"
		}
		body, ok := v.childOfKind(node, "declaration_list")
		if !ok {
			return found
		}
		for _, member := range v.children(body) {
			found = appendCPPNode(v, found, member, inner)
		}
		return found
	case "template_declaration":
		// The template header is part of the signature, but the declaration it
		// wraps is what carries the name.
		for _, child := range v.children(node) {
			found = appendCPPNode(v, found, child, prefix)
		}
		return found
	case "class_specifier", "struct_specifier", "union_specifier":
		identifier, ok := v.childOfKind(node, "type_identifier")
		if !ok {
			return found
		}
		name := prefix + v.text(identifier)
		found = append(found, declaration{name: name, kind: kindClass, raw: v.header(node, cBodies...)})
		body, ok := v.childOfKind(node, "field_declaration_list")
		if !ok {
			return found
		}
		// `struct` members are public until a label says otherwise; `class`
		// members are private until one does.
		public := v.kind(node) != "class_specifier"
		for _, member := range v.children(body) {
			if v.kind(member) == "access_specifier" {
				public = v.text(member) == "public" || v.text(member) == "public:"
				continue
			}
			if !public {
				continue
			}
			found = appendCPPMember(v, found, member, name+"::")
		}
		return found
	case "enum_specifier":
		return appendCNode(v, found, node, prefix)
	}
	return appendCNode(v, found, node, prefix)
}

func appendCPPMember(v view, found []declaration, node int, prefix string) []declaration {
	switch v.kind(node) {
	case "field_declaration", "declaration", "function_definition":
		declarator, ok := v.childOfKind(node, "function_declarator", "pointer_declarator", "field_identifier", "init_declarator", "identifier", "array_declarator", "reference_declarator")
		if !ok {
			return found
		}
		name, ok := cDeclaratorName(v, declarator)
		if !ok {
			return found
		}
		kind := kindField
		if cIsFunction(v, declarator) {
			kind = kindMethod
		}
		return append(found, declaration{name: prefix + name, kind: kind, raw: cHeader(v, node, declarator)})
	case "class_specifier", "struct_specifier", "enum_specifier", "template_declaration":
		return appendCPPNode(v, found, node, prefix)
	}
	return found
}
