package main

import "testing"

func TestMax(t *testing.T) {
	if Max(5, 3) != 5 {
		t.Error("failed")
	}
	if Max(3, 5) != 5 {
		t.Error("failed")
	}
}
