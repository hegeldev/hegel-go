package libhegel

import "testing"

func TestErrorStringBoundaries(t *testing.T) {
	for _, tc := range []struct {
		code Error
		want string
	}{
		{E_RETRY, "E_RETRY"},
		{OK, "OK"},
		{Error(-11), "Error(-11)"},
		{Error(1), "Error(1)"},
	} {
		if got := tc.code.String(); got != tc.want {
			t.Errorf("%d.String() = %q, want %q", tc.code, got, tc.want)
		}
	}
}
