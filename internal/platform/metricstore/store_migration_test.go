package metricstore

import "testing"

func TestMaskDSN(t *testing.T) {
	cases := map[string]string{
		"":                                  "",
		"mysql://user@host/db":              "mysql://user@host/db",
		"mysql://user:pass@host/db":         "mysql://user:***@host/db",
		"postgres://user:p@ss@host:5432/db": "postgres://user:***@host:5432/db",
		"user:pass@host":                    "user:***@host",
		"host=localhost password=secret dbname=x": "host=localhost password=*** dbname=x",
	}
	for in, want := range cases {
		if got := maskDSN(in); got != want {
			t.Errorf("maskDSN(%q) = %q, want %q", in, got, want)
		}
	}
}
