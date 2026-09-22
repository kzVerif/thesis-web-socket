package config

import "testing"

func TestProductionTransportConfiguration(t *testing.T) {
	valid := Config{TransportMode: "production", DatabaseURL: "test-only", Address: "127.0.0.1:8081", PublicBaseURL: "https://socket.example.com", FrontendOrigins: []string{"lab.example.com"}}
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*Config){
		func(c *Config) { c.Address = ":8081" },
		func(c *Config) { c.PublicBaseURL = "http://socket.example.com" },
		func(c *Config) { c.PublicBaseURL = "https://secret@socket.example.com" },
		func(c *Config) { c.FrontendOrigins = nil },
		func(c *Config) { c.FrontendOrigins = []string{"*"} },
		func(c *Config) { c.FrontendOrigins = []string{"*.example.com"} },
		func(c *Config) { c.FrontendOrigins = []string{"https://lab.example.com"} },
		func(c *Config) { c.TransportMode = "prod" },
	} {
		cfg := valid
		mutate(&cfg)
		if err := cfg.Validate(); err == nil {
			t.Fatal("unsafe configuration accepted")
		}
	}
	t.Setenv("TRANSPORT_MODE", "production")
	t.Setenv("DATABASE_URL", "test-only")
	t.Setenv("SERVER_ADDRESS", "127.0.0.1:8081")
	t.Setenv("PUBLIC_BASE_URL", "https://socket.example.com")
	t.Setenv("FRONTEND_ORIGINS", "")
	if err := Load().Validate(); err == nil {
		t.Fatal("production silently used development origin defaults")
	}
}
