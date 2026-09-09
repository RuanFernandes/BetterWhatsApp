package desktop

import "testing"

func TestNormalizeUnreadCount(t *testing.T) {
	tests := []struct {
		name  string
		input int
		want  int
	}{
		{name: "negative", input: -1, want: 0},
		{name: "zero", input: 0, want: 0},
		{name: "normal", input: 42, want: 42},
		{name: "upper bound", input: 1000000, want: 999999},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := normalizeUnreadCount(test.input); got != test.want {
				t.Fatalf("normalizeUnreadCount(%d) = %d, want %d", test.input, got, test.want)
			}
		})
	}
}

func TestUnreadLabel(t *testing.T) {
	if got := unreadLabel(0); got != "Nenhuma mensagem não lida" {
		t.Fatalf("unreadLabel(0) = %q", got)
	}
	if got := unreadLabel(1); got != "1 mensagem não lida" {
		t.Fatalf("unreadLabel(1) = %q", got)
	}
	if got := unreadLabel(3); got != "3 mensagens não lidas" {
		t.Fatalf("unreadLabel(3) = %q", got)
	}
}
