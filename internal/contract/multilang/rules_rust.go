package multilang

import "strings"

// Rust's exported surface is what `pub` marks. The visibility modifier covers
// pub, pub(crate), pub(super) and pub(in path); only bare `pub` is reachable
// from another crate, but a crate-visible item is still surface another
// workstream in the same crate depends on, so all of them count.

// rustBodies are the nodes that open an item body.
var rustBodies = []string{"block", "declaration_list", "field_declaration_list", "ordered_field_declaration_list", "enum_variant_list"}

func collectRust(v view) []declaration {
	if len(v.records) == 0 {
		return nil
	}
	var found []declaration
	for _, top := range v.children(0) {
		found = appendRustItem(v, found, top, "", false)
	}
	return found
}

func appendRustItem(v view, found []declaration, node int, prefix string, implicitlyPublic bool) []declaration {
	kind, ok := rustItemKind(v.kind(node))
	if !ok {
		return found
	}
	if !implicitlyPublic && !rustIsPublic(v, node) {
		return found
	}
	name, ok := rustName(v, node)
	if !ok {
		return found
	}
	qualified := prefix + name
	found = append(found, declaration{name: qualified, kind: kind, raw: rustHeader(v, node)})

	// A public trait's methods are surface; an inherent impl block's public
	// methods are too. A module's public items keep their path.
	switch v.kind(node) {
	case "trait_item", "mod_item", "impl_item":
		body, ok := v.childOfKind(node, rustBodies...)
		if !ok {
			return found
		}
		// A trait's methods carry no visibility of their own: they are as
		// public as the trait that declares them, exactly like Java's
		// interface members.
		implicit := v.kind(node) == "trait_item"
		for _, member := range v.children(body) {
			found = appendRustItem(v, found, member, qualified+"::", implicit)
		}
	}
	return found
}

// rustHeader stops a value item at its initializer, so changing a constant's
// value is not a contract change, and stops everything else at its body.
func rustHeader(v view, node int) string {
	switch v.kind(node) {
	case "const_item", "static_item", "type_item":
		if assign, ok := v.childOfKind(node, "="); ok {
			return strings.TrimRight(v.textTo(node, assign), " \t\r\n")
		}
		return v.text(node)
	}
	return v.header(node, rustBodies...)
}

func rustItemKind(node string) (string, bool) {
	switch node {
	case "function_item", "function_signature_item":
		return kindFunction, true
	case "struct_item", "union_item":
		return kindClass, true
	case "enum_item":
		return kindEnum, true
	case "trait_item":
		return kindInterface, true
	case "type_item":
		return kindAlias, true
	case "const_item", "static_item":
		return kindConst, true
	case "mod_item":
		return kindNamespace, true
	case "impl_item":
		return kindNamespace, true
	}
	return "", false
}

// rustIsPublic reports whether an item carries a visibility modifier. An impl
// block has none of its own — it is a container whose members carry theirs — so
// it is always walked into.
func rustIsPublic(v view, node int) bool {
	if v.kind(node) == "impl_item" {
		return true
	}
	modifier, ok := v.childOfKind(node, "visibility_modifier")
	if !ok {
		return false
	}
	return strings.HasPrefix(strings.TrimSpace(v.text(modifier)), "pub")
}

func rustName(v view, node int) (string, bool) {
	// An impl block is named by the type it implements, which is a type
	// identifier rather than an identifier.
	identifier, ok := v.childOfKind(node, "identifier", "type_identifier")
	if !ok {
		return "", false
	}
	name := v.text(identifier)
	if name == "" {
		return "", false
	}
	return name, true
}
