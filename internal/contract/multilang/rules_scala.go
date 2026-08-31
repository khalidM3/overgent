package multilang

import "strings"

// Scala declarations are public unless a modifier says otherwise, so the rule
// is exclusion by `private` or `protected` rather than inclusion by `public`.

// scalaBodies open a definition's body for the purpose of cutting a header.
var scalaBodies = []string{"template_body", "block"}

// scalaMemberBodies are the bodies whose contents are surface. A block is
// deliberately absent: a method body holds local values, and recording those
// would make renaming a local read as a changed public contract.
var scalaMemberBodies = []string{"template_body"}

func collectScala(v view) []declaration {
	if len(v.records) == 0 {
		return nil
	}
	var found []declaration
	for _, top := range v.children(0) {
		found = appendScalaNode(v, found, top, "")
	}
	return found
}

func appendScalaNode(v view, found []declaration, node int, prefix string) []declaration {
	kind, ok := scalaKind(v.kind(node))
	if !ok {
		return found
	}
	if scalaIsHidden(v, node) {
		return found
	}
	identifier, ok := v.childOfKind(node, "identifier", "type_identifier", "_")
	if !ok {
		return found
	}
	name := v.text(identifier)
	if name == "" {
		return found
	}
	qualified := prefix + name
	found = append(found, declaration{name: qualified, kind: kind, raw: scalaHeader(v, node)})

	body, ok := v.childOfKind(node, scalaMemberBodies...)
	if !ok {
		return found
	}
	for _, member := range v.children(body) {
		found = appendScalaNode(v, found, member, qualified+".")
	}
	return found
}

// scalaHeader drops a definition's right-hand side, so a changed body or value
// is not a changed contract.
//
// The `=` is checked before the body because Scala writes the same definition
// two ways — `def f: Int = x` and `def f: Int = { … }` — and only the second
// has a block. Cutting at the block would leave the `=` in one signature and
// not the other, so reformatting a method into a block would read as a changed
// contract. A class or trait has no `=` and stops at its template body; a
// default parameter's `=` is nested inside the parameter list and is never a
// direct child.
func scalaHeader(v view, node int) string {
	if equals, ok := v.childOfKind(node, "="); ok {
		return strings.TrimRight(v.textTo(node, equals), " \t\r\n")
	}
	if body, ok := v.childOfKind(node, scalaBodies...); ok {
		return strings.TrimRight(v.textTo(node, body), " \t\r\n")
	}
	return v.text(node)
}

func scalaKind(node string) (string, bool) {
	switch node {
	case "class_definition", "case_class_definition":
		return kindClass, true
	case "object_definition":
		return kindNamespace, true
	case "trait_definition":
		return kindInterface, true
	case "function_definition", "function_declaration":
		return kindFunction, true
	case "val_definition", "val_declaration", "var_definition", "var_declaration":
		return kindConst, true
	case "type_definition":
		return kindAlias, true
	}
	return "", false
}

func scalaIsHidden(v view, node int) bool {
	modifiers, ok := v.childOfKind(node, "modifiers")
	if !ok {
		return false
	}
	text := v.text(modifiers)
	return strings.Contains(text, "private") || strings.Contains(text, "protected")
}
