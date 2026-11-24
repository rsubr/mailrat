package mail

import "testing"

func TestRepairMojibake(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"Coimbatore â€\u201c 641 035", "Coimbatore – 641 035"},
		{"Donâ€™t Â© 2024 cafÃ©", "Don’t © 2024 café"},
		{"plain ASCII", "plain ASCII"},
		{"already fine: café – ©", "already fine: café – ©"},
		{"Ã", "Ã"}, // lone lead character is not a sequence
	} {
		if got := RepairMojibake(c.in); got != c.want {
			t.Errorf("RepairMojibake(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
