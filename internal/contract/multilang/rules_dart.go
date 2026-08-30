package multilang

import "strings"

// Dart marks privacy with a leading underscore, the same convention Python
// uses. There is no keyword, so the name is the whole signal.
//
// Dart's tree is unusual: a method's body is a sibling of its signature rather
// than a child, so a signature node already excludes the body and needs no
// truncation.

func collectDart(v view) []declaration {
	if len(v.records) == 0 {
		return nil
	}
	var found []declaration
	for _, top := range v.children(0) {
		found = appendDartNode(v, found, top, "")
	}
	return found
}

func appendDartNode(v view, found []declaration, node int, prefix string) []declaration {
	switch v.kind(node) {
	case "class_definition", "mixin_declaration", "extension_declaration", "enum_declaration":
		identifier, ok := v.childOfKind(node, "identifier")
		if !ok {
			return found
		}
		name := v.text(identifier)
		if !dartExported(name) {
			return found
		}
		kind := kindClass
		if v.kind(node) == "enum_declaration" {
			kind = kindEnum
		}
		qualified := prefix + name
		found = append(found, declaration{name: qualified, kind: kind, raw: v.header(node, "class_body", "enum_body")})
		body, ok := v.childOfKind(node, "class_body", "enum_body")
		if !ok {
			return found
		}
		for _, member := range v.children(body) {
			found = appendDartMember(v, found, member, qualified+".")
		}
		return found
	case "function_signature":
		name, ok := dartName(v, node)
		if !ok {
			return found
		}
		return append(found, declaration{name: prefix + name, kind: kindFunction, raw: v.text(node)})
	}
	return found
}

func appendDartMember(v view, found []declaration, node int, prefix string) []declaration {
	switch v.kind(node) {
	case "method_signature":
		signature, ok := v.childOfKind(node, "function_signature", "getter_signature", "setter_signature", "constructor_signature", "factory_constructor_signature")
		if !ok {
			return found
		}
		name, ok := dartName(v, signature)
		if !ok {
			return found
		}
		return append(found, declaration{name: prefix + name, kind: kindMethod, raw: v.text(node)})
	case "declaration":
		// A field declaration names itself through an identifier list; the
		// initializer is not part of the contract.
		list, ok := v.childOfKind(node, "initialized_identifier_list", "static_final_declaration_list", "initialized_variable_definition")
		if !ok {
			return found
		}
		name, ok := dartName(v, list)
		if !ok {
			return found
		}
		return append(found, declaration{name: prefix + name, kind: kindField, raw: dartFieldHeader(v, node)})
	}
	return found
}

func dartFieldHeader(v view, node int) string {
	if equals, ok := v.childOfKind(node, "="); ok {
		return strings.TrimRight(v.textTo(node, equals), " \t\r\n")
	}
	return v.text(node)
}

func dartName(v view, node int) (string, bool) {
	identifier, ok := v.childOfKind(node, "identifier")
	if !ok {
		// An identifier list wraps each name in an initialized_identifier, so
		// the name is a grandchild rather than a child.
		wrapper, wrapped := v.childOfKind(node, "initialized_identifier")
		if !wrapped {
			return "", false
		}
		if identifier, ok = v.childOfKind(wrapper, "identifier"); !ok {
			return "", false
		}
	}
	name := v.text(identifier)
	if name == "" || !dartExported(name) {
		return "", false
	}
	return name, true
}

// dartExported applies Dart's convention: a leading underscore is library
// private.
func dartExported(name string) bool {
	return name != "" && !strings.HasPrefix(name, "_")
}
