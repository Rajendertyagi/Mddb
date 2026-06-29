package sliceutil

import (
	"reflect"
	"testing"
)

func TestUnique(t *testing.T) {
	cases := []struct {
		name string
		in   []string
		want []string
	}{
		{"empty", nil, []string{}},
		{"no dups", []string{"a", "b", "c"}, []string{"a", "b", "c"}},
		{"dups preserve first-seen order", []string{"b", "a", "b", "c", "a"}, []string{"b", "a", "c"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Unique(tc.in); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("Unique(%v) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestUniqueIntsDoesNotMutateInput(t *testing.T) {
	in := []int{1, 1, 2, 3, 3}
	_ = Unique(in)
	if !reflect.DeepEqual(in, []int{1, 1, 2, 3, 3}) {
		t.Errorf("Unique mutated its input: %v", in)
	}
	if got := Unique(in); !reflect.DeepEqual(got, []int{1, 2, 3}) {
		t.Errorf("Unique(ints) = %v, want [1 2 3]", got)
	}
}
