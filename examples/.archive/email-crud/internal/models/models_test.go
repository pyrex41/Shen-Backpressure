package models

import "testing"

func TestUserIsKnown(t *testing.T) {
	age := 30
	state := "CA"

	tests := []struct {
		name string
		user User
		want bool
	}{
		{
			name: "both set",
			user: User{ID: "1", Email: "a@b.com", AgeDecade: &age, State: &state},
			want: true,
		},
		{
			name: "age nil",
			user: User{ID: "2", Email: "b@b.com", AgeDecade: nil, State: &state},
			want: false,
		},
		{
			name: "state nil",
			user: User{ID: "3", Email: "c@b.com", AgeDecade: &age, State: nil},
			want: false,
		},
		{
			name: "both nil",
			user: User{ID: "4", Email: "d@b.com"},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.user.IsKnown(); got != tt.want {
				t.Errorf("IsKnown() = %v, want %v", got, tt.want)
			}
		})
	}
}
