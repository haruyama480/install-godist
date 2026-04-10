package main

import "testing"

func TestResolveVersion(t *testing.T) {
	releases := []GoRelease{
		{Version: "go1.23.2"},
		{Version: "go1.23.1"},
		{Version: "go1.22.5"},
		{Version: "go1.22"},
		{Version: "go1.21.9"},
	}

	tests := []struct {
		name    string
		target  string
		want    string
		wantNil bool
	}{
		{
			name:   "latest returns first release",
			target: "latest",
			want:   "go1.23.2",
		},
		{
			name:   "minor only returns first matching patch",
			target: "1.22",
			want:   "go1.22.5",
		},
		{
			name:   "exact version match",
			target: "1.23.1",
			want:   "go1.23.1",
		},
		{
			name:    "not found returns nil",
			target:  "1.20.1",
			wantNil: true,
		},
		{
			name:   "minor can match exact minor tag",
			target: "1.22",
			want:   "go1.22.5",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := resolveVersion(tc.target, releases)
			if tc.wantNil {
				if got != nil {
					t.Fatalf("expected nil, got %v", got.Version)
				}
				return
			}

			if got == nil {
				t.Fatalf("expected %s, got nil", tc.want)
			}
			if got.Version != tc.want {
				t.Fatalf("expected %s, got %s", tc.want, got.Version)
			}
		})
	}
}

func TestResolveVersion_LatestWithEmptyReleasesPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatalf("expected panic, but did not panic")
		}
	}()

	_ = resolveVersion("latest", nil)
}
