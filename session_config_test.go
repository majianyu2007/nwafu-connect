package main
import "testing"
func TestSessionRefreshConfiguration(t *testing.T) {
	file := writeConfig(t, "session_refresh_interval = 900\n")
	for _, tt := range []struct {
		name string
		args []string
		env  []string
		want int
	}{
		{"default", nil, nil, 1800},
		{"file", []string{"--config", file}, nil, 900},
		{"environment", []string{"--config", file}, []string{"NWAFU_CONNECT_SESSION_REFRESH_INTERVAL=600"}, 600},
		{"cli", []string{"--config", file, "-session-refresh-interval", "300"}, []string{"NWAFU_CONNECT_SESSION_REFRESH_INTERVAL=600"}, 300},
		{"disabled", []string{"--config", file, "--session-refresh-interval=0"}, nil, 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			options, _, err := loadStartupOptions(tt.args, func() []string { return tt.env })
			if err != nil {
				t.Fatal(err)
			}
			if got := options.Config.SessionRefreshInterval; got != tt.want {
				t.Fatalf("session refresh seconds = %v, want %v", got, tt.want)
			}
		})
	}
}

