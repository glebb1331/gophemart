package config

import (
	"flag"

	"github.com/ilyakaznacheev/cleanenv"
)

type Config struct {
	RunAddress           string `env:"RUN_ADDRESS" env-default:"localhost:8080"`
	DatabaseURI          string `env:"DATABASE_URI"`
	AccrualSystemAddress string `env:"ACCRUAL_SYSTEM_ADDRESS"`
}

func NewConfig() (*Config, error) {
	cfg := &Config{}

	flag.StringVar(&cfg.RunAddress, "a", "localhost:8080", "run address")
	flag.StringVar(&cfg.DatabaseURI, "db", "", "database URI")
	flag.StringVar(&cfg.AccrualSystemAddress, "r", "", "accrual system address")
	flag.Parse()

	if err := cleanenv.ReadEnv(cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}
