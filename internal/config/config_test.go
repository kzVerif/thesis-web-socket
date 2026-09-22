package config

import "testing"

func TestDatabaseConfigurationRequired(t *testing.T) {
	for _, value := range []string{"", " \t\n"} {
		t.Setenv("DATABASE_URL", value)
		if err := Load().Validate(); err == nil {
			t.Fatal("missing database configuration accepted")
		}
	}
	t.Setenv("DATABASE_URL", "postgres://test-user:example-password@localhost/test-only")
	cfg := Load()
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	if cfg.DatabaseURL != "postgres://test-user:example-password@localhost/test-only" {
		t.Fatal("configured DSN was changed")
	}
}
