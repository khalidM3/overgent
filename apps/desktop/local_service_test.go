package main

import "testing"

func TestIntegerDecodesDaemonJSONNumbers(t *testing.T) {
	if integer(float64(7)) != 7 || integer("7") != 0 {
		t.Fatal("daemon number conversion was not fail-closed")
	}
}
