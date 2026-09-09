package appservice

import (
	"context"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"
)

type testWindow struct {
	name string
}

func (w testWindow) Name() string {
	return w.name
}

func TestRequireControlWindow(t *testing.T) {
	tests := []struct {
		name    string
		ctx     context.Context
		wantErr bool
	}{
		{
			name:    "nil context",
			wantErr: true,
		},
		{
			name:    "missing window",
			ctx:     context.Background(),
			wantErr: true,
		},
		{
			name: "remote document",
			ctx: context.WithValue(
				context.Background(),
				application.WindowKey,
				testWindow{name: "web.whatsapp.com"},
			),
			wantErr: true,
		},
		{
			name: "legacy settings window",
			ctx: context.WithValue(
				context.Background(),
				application.WindowKey,
				testWindow{name: "settings"},
			),
			wantErr: true,
		},
		{
			name: "local control surface",
			ctx: context.WithValue(
				context.Background(),
				application.WindowKey,
				testWindow{name: controlWindowName},
			),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := requireControlWindow(test.ctx)
			if (err != nil) != test.wantErr {
				t.Fatalf("requireControlWindow() error = %v, wantErr %v", err, test.wantErr)
			}
		})
	}
}
