package contract_test

import (
	"testing"

	"github.com/stickguy/stickguy/internal/contract"
)

// Each language added under ADR-063 has to get one thing right above all
// others: what is public. A non-public declaration recorded as contract surface
// produces a stale-assumption finding for work no other session can even see,
// which is a false interruption. These tests pin the visibility rule per
// language, and pin that a body-only edit never moves the hash.

func TestJavaRecordsOnlyPublicSurface(t *testing.T) {
	source := []byte(`package app;

public class Session {
    public static final String NAME = "session";
    private String token;
    public Session(String token) { this.token = token; }
    public String refresh(String policy) { return token; }
    private void internal() {}
    protected void guarded() {}
}

class PackagePrivate { public void invisible() {} }

public interface Api { String read(int id); }
`)
	file, ok := contract.Extract("app/Session.java", source, nil)
	if !ok {
		t.Fatal("java source produced no fingerprint")
	}
	names := symbolNames(file)
	for _, want := range []string{"Session", "Session.NAME", "Session.refresh", "Api", "Api.read"} {
		if !slicesContains(names, want) {
			t.Fatalf("missing %s in %v", want, names)
		}
	}
	// A package-private class, and every non-public member, is invisible to
	// another workstream and must not be contract surface.
	for _, unwanted := range []string{"PackagePrivate", "PackagePrivate.invisible", "Session.token", "Session.internal", "Session.guarded"} {
		if slicesContains(names, unwanted) {
			t.Fatalf("non-public %s recorded: %v", unwanted, names)
		}
	}
}

func TestRustRecordsOnlyPubSurface(t *testing.T) {
	source := []byte(`pub struct Session { pub id: String }
pub const MAX: u32 = 5;
pub fn rotate(session_id: &str) -> Result<(), Error> { Ok(()) }
fn private_helper() {}
pub trait Store { fn get(&self, k: &str) -> Option<String>; }
pub type Alias = Vec<u8>;
struct Hidden;
`)
	file, ok := contract.Extract("src/session.rs", source, nil)
	if !ok {
		t.Fatal("rust source produced no fingerprint")
	}
	names := symbolNames(file)
	// A trait's methods carry no visibility of their own and are as public as
	// the trait itself.
	for _, want := range []string{"Session", "MAX", "rotate", "Store", "Store::get", "Alias"} {
		if !slicesContains(names, want) {
			t.Fatalf("missing %s in %v", want, names)
		}
	}
	for _, unwanted := range []string{"private_helper", "Hidden"} {
		if slicesContains(names, unwanted) {
			t.Fatalf("private %s recorded: %v", unwanted, names)
		}
	}
}

func TestCSharpRecordsOnlyPublicSurface(t *testing.T) {
	source := []byte(`namespace App {
  public class Session {
    public string Name { get; set; }
    public string Refresh(string policy) { return policy; }
    private void Internal() {}
  }
  class Hidden { public void Nope() {} }
  public interface IApi { string Read(int id); }
}
`)
	file, ok := contract.Extract("App/Session.cs", source, nil)
	if !ok {
		t.Fatal("c# source produced no fingerprint")
	}
	names := symbolNames(file)
	// The namespace is a path, not surface of its own, but it qualifies names.
	for _, want := range []string{"App.Session", "App.Session.Name", "App.Session.Refresh", "App.IApi", "App.IApi.Read"} {
		if !slicesContains(names, want) {
			t.Fatalf("missing %s in %v", want, names)
		}
	}
	for _, unwanted := range []string{"App.Hidden", "App.Session.Internal"} {
		if slicesContains(names, unwanted) {
			t.Fatalf("non-public %s recorded: %v", unwanted, names)
		}
	}
}

func TestPHPTreatsAbsentModifierAsPublic(t *testing.T) {
	source := []byte(`<?php
function rotate(string $sessionId): bool { return true; }
class Session {
    public const NAME = 'session';
    private $token;
    public function refresh(string $policy): string { return $policy; }
    private function internal(): void {}
    protected function guarded(): void {}
}
interface Api { public function read(int $id): string; }
`)
	file, ok := contract.Extract("src/Session.php", source, nil)
	if !ok {
		t.Fatal("php source produced no fingerprint")
	}
	names := symbolNames(file)
	// PHP has no file-private declaration, so a top-level function is surface
	// with no modifier at all.
	for _, want := range []string{"rotate", "Session", "Session::NAME", "Session::refresh", "Api", "Api::read"} {
		if !slicesContains(names, want) {
			t.Fatalf("missing %s in %v", want, names)
		}
	}
	for _, unwanted := range []string{"Session::internal", "Session::guarded"} {
		if slicesContains(names, unwanted) {
			t.Fatalf("non-public %s recorded: %v", unwanted, names)
		}
	}
}

func TestNewLanguagesIgnoreBodyOnlyEdits(t *testing.T) {
	for _, testCase := range []struct{ path, before, after string }{
		{
			"A.java",
			"public class A { public int f(int x) { return x; } }",
			"public class A { public int f(int x) { // rewritten\n int y = x * 2; return y; } }",
		},
		{
			"a.rs",
			"pub fn f(x: u32) -> u32 { x }",
			"pub fn f(x: u32) -> u32 { // rewritten\n let y = x * 2; y }",
		},
		{
			"A.cs",
			"public class A { public int F(int x) { return x; } }",
			"public class A { public int F(int x) { // rewritten\n var y = x * 2; return y; } }",
		},
		{
			"a.php",
			"<?php\nfunction f(int $x): int { return $x; }\n",
			"<?php\nfunction f(int $x): int { // rewritten\n $y = $x * 2; return $y; }\n",
		},
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
			t.Fatalf("%s: body-only edit moved the hash", testCase.path)
		}
	}
}

func TestNewLanguageSignatureChangesMoveTheHash(t *testing.T) {
	for _, testCase := range []struct{ path, before, after string }{
		{"A.java", "public class A { public int f(int x) { return x; } }", "public class A { public int f(int x, String p) { return x; } }"},
		{"a.rs", "pub fn f(x: u32) -> u32 { x }", "pub fn f(x: u32, p: &str) -> u32 { x }"},
		{"A.cs", "public class A { public int F(int x) { return x; } }", "public class A { public int F(int x, string p) { return x; } }"},
		{"a.php", "<?php\nfunction f(int $x): int { return $x; }\n", "<?php\nfunction f(int $x, string $p): int { return $x; }\n"},
	} {
		before, _ := contract.Extract(testCase.path, []byte(testCase.before), nil)
		after, ok := contract.Extract(testCase.path, []byte(testCase.after), nil)
		if !ok {
			t.Fatalf("%s: no fingerprint after the signature change", testCase.path)
		}
		if before.FileContractHash == after.FileContractHash {
			t.Fatalf("%s: a changed signature must move the hash", testCase.path)
		}
	}
}
