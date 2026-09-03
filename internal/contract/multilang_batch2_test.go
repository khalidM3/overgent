package contract_test

import (
	"strings"
	"testing"

	"github.com/overgent/overgent/internal/contract"
)

// The second language batch (ADR-063). As with the first, the property that
// matters most is what counts as public: recording an unreachable declaration
// as contract surface produces a stale-assumption finding for work no other
// session can see.

func TestCRecordsNonStaticSurface(t *testing.T) {
	source := []byte(`#include <stdio.h>
int rotate(const char *id, int policy);
static int helper(void) { return 1; }
int rotate(const char *id, int policy) { return 0; }
struct Session { int id; };
typedef struct Session Sess;
int global_count = 3;
static int private_count = 4;
`)
	file, ok := contract.Extract("src/session.c", source, nil)
	if !ok {
		t.Fatal("c source produced no fingerprint")
	}
	names := symbolNames(file)
	for _, want := range []string{"rotate", "Session", "Sess", "global_count"} {
		if !slicesContains(names, want) {
			t.Fatalf("missing %s in %v", want, names)
		}
	}
	// static gives internal linkage: nothing outside this translation unit can
	// reach it, so it is not contract surface.
	for _, unwanted := range []string{"helper", "private_count"} {
		if slicesContains(names, unwanted) {
			t.Fatalf("static %s recorded: %v", unwanted, names)
		}
	}
	// A prototype and its definition are one contract, not two.
	seen := 0
	for _, name := range names {
		if name == "rotate" {
			seen++
		}
	}
	if seen != 1 {
		t.Fatalf("rotate recorded %d times, want once: %v", seen, names)
	}
}

func TestCPPHonorsAccessSections(t *testing.T) {
	source := []byte(`namespace app {
class Session {
public:
    int refresh(const char *policy);
    int count;
private:
    int hidden;
    void internal();
};
int rotate(int id);
static int helper();
}
`)
	file, ok := contract.Extract("src/session.cpp", source, nil)
	if !ok {
		t.Fatal("c++ source produced no fingerprint")
	}
	names := symbolNames(file)
	for _, want := range []string{"app::Session", "app::Session::refresh", "app::Session::count", "app::rotate"} {
		if !slicesContains(names, want) {
			t.Fatalf("missing %s in %v", want, names)
		}
	}
	// Access is positional: everything after `private:` is hidden, and a class
	// starts private before any label at all.
	for _, unwanted := range []string{"app::Session::hidden", "app::Session::internal", "app::helper"} {
		if slicesContains(names, unwanted) {
			t.Fatalf("non-public %s recorded: %v", unwanted, names)
		}
	}
}

func TestCPPClassDefaultsPrivateAndStructDefaultsPublic(t *testing.T) {
	class, ok := contract.Extract("a.cpp", []byte("class A {\n    int hidden;\n};\n"), nil)
	if !ok {
		t.Fatal("no fingerprint for the class")
	}
	if slicesContains(symbolNames(class), "A::hidden") {
		t.Fatalf("an unlabelled class member must be private: %v", symbolNames(class))
	}
	strukt, ok := contract.Extract("b.cpp", []byte("struct B {\n    int shown;\n};\n"), nil)
	if !ok {
		t.Fatal("no fingerprint for the struct")
	}
	if !slicesContains(symbolNames(strukt), "B::shown") {
		t.Fatalf("an unlabelled struct member must be public: %v", symbolNames(strukt))
	}
}

func TestScalaAndKotlinExcludeOnlyMarkedPrivates(t *testing.T) {
	scala, ok := contract.Extract("src/Session.scala", []byte(`package app
class Session(val id: String) {
  def refresh(policy: String): String = policy
  private def internal(): Unit = ()
  val name: String = "s"
}
object Registry { def get(k: String): Option[String] = None }
trait Store { def put(k: String): Unit }
private class Hidden { def nope(): Unit = () }
`), nil)
	if !ok {
		t.Fatal("scala source produced no fingerprint")
	}
	for _, want := range []string{"Session", "Session.refresh", "Session.name", "Registry", "Store"} {
		if !slicesContains(symbolNames(scala), want) {
			t.Fatalf("missing scala %s in %v", want, symbolNames(scala))
		}
	}
	for _, unwanted := range []string{"Session.internal", "Hidden"} {
		if slicesContains(symbolNames(scala), unwanted) {
			t.Fatalf("private scala %s recorded: %v", unwanted, symbolNames(scala))
		}
	}

	kotlin, ok := contract.Extract("src/Session.kt", []byte(`package app

class Session(val id: String) {
    fun refresh(policy: String): String = policy
    private fun internal() {}
    val name: String = "s"
}

private class Hidden {
    fun nope() {}
}

fun topLevel(a: Int): Int = a
`), nil)
	if !ok {
		t.Fatal("kotlin source produced no fingerprint")
	}
	for _, want := range []string{"Session", "Session.refresh", "Session.name", "topLevel"} {
		if !slicesContains(symbolNames(kotlin), want) {
			t.Fatalf("missing kotlin %s in %v", want, symbolNames(kotlin))
		}
	}
	for _, unwanted := range []string{"Session.internal", "Hidden"} {
		if slicesContains(symbolNames(kotlin), unwanted) {
			t.Fatalf("private kotlin %s recorded: %v", unwanted, symbolNames(kotlin))
		}
	}
}

func TestDartUsesTheUnderscoreConvention(t *testing.T) {
	source := []byte(`class Session {
  String id;
  String refresh(String policy) => policy;
  void _internal() {}
}
int topLevel(int a) => a;
void _hidden() {}
`)
	file, ok := contract.Extract("lib/session.dart", source, nil)
	if !ok {
		t.Fatal("dart source produced no fingerprint")
	}
	names := symbolNames(file)
	for _, want := range []string{"Session", "Session.id", "Session.refresh", "topLevel"} {
		if !slicesContains(names, want) {
			t.Fatalf("missing %s in %v", want, names)
		}
	}
	// Dart has no visibility keyword: a leading underscore is library private.
	for _, unwanted := range []string{"Session._internal", "_hidden"} {
		if slicesContains(names, unwanted) {
			t.Fatalf("private %s recorded: %v", unwanted, names)
		}
	}
}

func TestSecondBatchIgnoresBodyOnlyEdits(t *testing.T) {
	for _, testCase := range []struct{ path, before, after string }{
		{"a.c", "int f(int x) { return x; }\n", "int f(int x) {\n    /* rewritten */\n    int y = x * 2;\n    return y;\n}\n"},
		{"a.cpp", "struct A {\n    int f(int x) { return x; }\n};\n", "struct A {\n    int f(int x) {\n        // rewritten\n        return x * 2;\n    }\n};\n"},
		{"A.scala", "class A {\n  def f(x: Int): Int = x\n}\n", "class A {\n  def f(x: Int): Int = {\n    val y = x * 2\n    y\n  }\n}\n"},
		{"A.kt", "class A {\n    fun f(x: Int): Int = x\n}\n", "class A {\n    fun f(x: Int): Int {\n        return x * 2\n    }\n}\n"},
	} {
		before, ok := contract.Extract(testCase.path, []byte(testCase.before), nil)
		if !ok {
			t.Fatalf("%s: no fingerprint for the original", testCase.path)
		}
		after, ok := contract.Extract(testCase.path, []byte(testCase.after), nil)
		if !ok {
			t.Fatalf("%s: no fingerprint after the body edit", testCase.path)
		}
		if before.FileContractHash != after.FileContractHash {
			t.Errorf("%s: body-only edit moved the hash", testCase.path)
		}
	}
}

// TestLocalsAreNeverContractSurface pins a property that is easy to break by
// walking one node too deep. A local variable is not API, and recording one
// would make renaming it inside a method body read as a changed public
// contract — a false interruption for work nobody else can see. Scala did
// exactly this until its member recursion was narrowed to template bodies.
func TestLocalsAreNeverContractSurface(t *testing.T) {
	for _, testCase := range []struct{ path, source, local string }{
		{"a.py", "def f():\n    local = 5\n    return local\n", "local"},
		{"a.js", "export function f() {\n  const local = 5;\n  return local;\n}\n", "local"},
		{"A.java", "public class A {\n    public int f() {\n        int local = 5;\n        return local;\n    }\n}\n", "local"},
		{"A.cs", "public class A {\n    public int F() {\n        var local = 5;\n        return local;\n    }\n}\n", "local"},
		{"a.php", "<?php\nfunction f() {\n    $local = 5;\n    return $local;\n}\n", "local"},
		{"a.rs", "pub fn f() -> u32 {\n    let local = 5;\n    local\n}\n", "local"},
		{"a.c", "int f() {\n    int local = 5;\n    return local;\n}\n", "local"},
		{"a.cpp", "struct A {\n    int f() {\n        int local = 5;\n        return local;\n    }\n};\n", "local"},
		{"A.scala", "class A {\n  def f(x: Int): Int = {\n    val local = x * 2\n    local\n  }\n}\n", "local"},
		{"A.kt", "fun f(): Int {\n    val local = 5\n    return local\n}\n", "local"},
		{"a.dart", "int f() {\n  var local = 5;\n  return local;\n}\n", "local"},
	} {
		file, ok := contract.Extract(testCase.path, []byte(testCase.source), nil)
		if !ok {
			t.Errorf("%s: no fingerprint", testCase.path)
			continue
		}
		for _, name := range symbolNames(file) {
			if name == testCase.local || strings.HasSuffix(name, "."+testCase.local) || strings.HasSuffix(name, "::"+testCase.local) {
				t.Errorf("%s: local %q recorded as contract surface: %v", testCase.path, testCase.local, symbolNames(file))
			}
		}
	}
}
