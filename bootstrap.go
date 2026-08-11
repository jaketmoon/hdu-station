package main

import (
	"fmt"
	"os"

	"github.com/jaketmoon/hdu-station/internal/appdata"
	"github.com/jaketmoon/hdu-station/internal/config"
	"github.com/joho/godotenv"
)

func createApplication() (*App, error) {
	root, err := appdata.ResolveRoot()
	if err != nil {
		return nil, err
	}
	values, err := readOptionalEnvironment(".env")
	if err != nil {
		return nil, err
	}
	lookup := func(key string) string {
		if value, ok := os.LookupEnv(key); ok {
			return value
		}
		return values[key]
	}
	return createApplicationAt(root, config.FromEnvironment(lookup))
}

func createApplicationAt(root string, seed config.Config) (*App, error) {
	store := config.NewStore(root)
	cfg, err := store.LoadOrCreate(seed)
	if err != nil {
		return nil, fmt.Errorf("load Station configuration: %w", err)
	}
	return NewApp(root, cfg), nil
}

func readOptionalEnvironment(path string) (map[string]string, error) {
	values, err := godotenv.Read(path)
	if err == nil {
		return values, nil
	}
	if os.IsNotExist(err) {
		return map[string]string{}, nil
	}
	return nil, fmt.Errorf("read development environment: %w", err)
}
