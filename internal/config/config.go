// Package config предоставляет конфигурацию сервиса лояльности Гофермарт.
// Конфигурация загружается из переменных окружения и флагов командной строки.
package config

import (
	"flag"

	"github.com/ilyakaznacheev/cleanenv"
)

// Config содержит параметры конфигурации сервиса.
type Config struct {
	// RunAddress — адрес и порт запуска HTTP-сервера.
	RunAddress string `env:"RUN_ADDRESS" env-default:"localhost:8080"`
	// DatabaseURI — строка подключения к базе данных PostgreSQL.
	DatabaseURI string `env:"DATABASE_URI"`
	// AccrualSystemAddress — адрес системы расчёта начислений баллов лояльности.
	AccrualSystemAddress string `env:"ACCRUAL_SYSTEM_ADDRESS"`
}

// NewConfig создаёт и возвращает конфигурацию сервиса.
// Приоритет: переменные окружения, затем флаги командной строки (если явно переданы).
func NewConfig() (*Config, error) {
	cfg := &Config{}

	if err := cleanenv.ReadEnv(cfg); err != nil {
		return nil, err
	}

	var flagAddr, flagDB, flagAccrual string
	flag.StringVar(&flagAddr, "a", "", "run address")
	flag.StringVar(&flagDB, "d", "", "database URI")
	flag.StringVar(&flagAccrual, "r", "", "accrual system address")
	flag.Parse()

	// Флаги перезаписывают только если явно переданы
	flag.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "a":
			cfg.RunAddress = flagAddr
		case "d":
			cfg.DatabaseURI = flagDB
		case "r":
			cfg.AccrualSystemAddress = flagAccrual
		}
	})

	return cfg, nil
}
