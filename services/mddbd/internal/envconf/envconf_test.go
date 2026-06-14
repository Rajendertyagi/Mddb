package envconf

import "testing"

func TestString(t *testing.T) {
	const key = "MDDB_ENVCONF_TEST_STRING"
	t.Run("returns default when unset", func(t *testing.T) {
		t.Setenv(key, "")
		if got := String(key, "fallback"); got != "fallback" {
			t.Fatalf("String() = %q, want %q", got, "fallback")
		}
	})
	t.Run("returns value when set", func(t *testing.T) {
		t.Setenv(key, "actual")
		if got := String(key, "fallback"); got != "actual" {
			t.Fatalf("String() = %q, want %q", got, "actual")
		}
	})
}

func TestInt(t *testing.T) {
	const key = "MDDB_ENVCONF_TEST_INT"
	cases := []struct {
		name string
		set  bool
		val  string
		def  int
		want int
	}{
		{"unset uses default", false, "", 7, 7},
		{"empty uses default", true, "", 7, 7},
		{"valid parses", true, "42", 7, 42},
		{"unparseable uses default", true, "notanumber", 7, 7},
		{"negative parses", true, "-3", 7, -3},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if c.set {
				t.Setenv(key, c.val)
			} else {
				t.Setenv(key, "")
			}
			if got := Int(key, c.def); got != c.want {
				t.Fatalf("Int(%q) = %d, want %d", c.val, got, c.want)
			}
		})
	}
}

func TestInt64(t *testing.T) {
	const key = "MDDB_ENVCONF_TEST_INT64"
	cases := []struct {
		name string
		val  string
		def  int64
		want int64
	}{
		{"empty uses default", "", 100, 100},
		{"valid parses", "9000000000", 100, 9000000000},
		{"unparseable uses default", "x", 100, 100},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv(key, c.val)
			if got := Int64(key, c.def); got != c.want {
				t.Fatalf("Int64(%q) = %d, want %d", c.val, got, c.want)
			}
		})
	}
}
