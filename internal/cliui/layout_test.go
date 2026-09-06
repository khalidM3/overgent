package cliui

import (
	"bytes"
	"reflect"
	"testing"
)

func TestWrapPreservesParagraphsAndHardWraps(t *testing.T) {
	want := []string{"coordina", "tion", "needs", "you", "", "abcdefgh", "ij"}
	if got := Wrap("coordination needs you\n\nabcdefghij", 8); !reflect.DeepEqual(got, want) {
		t.Fatalf("Wrap = %#v, want %#v", got, want)
	}
	if got := Wrap("e\u0301lan", 4); len(got) != 1 || got[0] != "e\u0301lan" {
		t.Fatalf("combining mark wrap = %#v", got)
	}
}

func TestWriteFieldsUsesAlignedAndStackedLayouts(t *testing.T) {
	fields := []Field{
		{Label: "Project", Value: "Acme API"},
		{Label: "Coordination", Value: "A contract changed after Checkout read it"},
	}
	var wide bytes.Buffer
	if err := WriteFields(&wide, 64, fields); err != nil {
		t.Fatal(err)
	}
	wantWide := "Project       Acme API\nCoordination  A contract changed after Checkout read it\n"
	if got := wide.String(); got != wantWide {
		t.Fatalf("wide fields:\n%q\nwant:\n%q", got, wantWide)
	}

	var narrow bytes.Buffer
	if err := WriteFields(&narrow, 24, fields[:1]); err != nil {
		t.Fatal(err)
	}
	wantNarrow := "Project\n  Acme API\n"
	if got := narrow.String(); got != wantNarrow {
		t.Fatalf("narrow fields = %q, want %q", got, wantNarrow)
	}
}
