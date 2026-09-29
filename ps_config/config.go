package ps_config

import (
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"

	"github.com/go-playground/validator/v10"
	"github.com/spf13/viper"
)

func New(cfg any) error {
	return newWithPaths(cfg, []string{"config.yaml", "config.local.yaml"})
}

func newWithPaths(cfg any, paths []string) error {
	v := viper.New()

	v.SetConfigType("yaml")
	for _, path := range paths {
		v.SetConfigFile(path)
		if err := v.MergeInConfig(); err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				return err
			}
		}
	}

	if err := rejectUnknownNonOTELEntries(v, cfg); err != nil {
		return err
	}

	if err := bindTags(v, cfg); err != nil {
		return err
	}

	if err := v.Unmarshal(cfg); err != nil {
		return err
	}

	if err := validator.New().Struct(cfg); err != nil {
		return err
	}

	if err := exportEnv(cfg); err != nil {
		return err
	}

	return nil
}

func rejectUnknownNonOTELEntries(v *viper.Viper, cfg any) error {
	knownKeys := map[string]struct{}{}
	configType := reflect.TypeOf(cfg)
	if configType.Kind() == reflect.Pointer {
		configType = configType.Elem()
	}
	for field := range configType.Fields() {
		if key := field.Tag.Get("mapstructure"); key != "" {
			knownKeys[key] = struct{}{}
		}
	}

	for _, key := range v.AllKeys() {
		if _, ok := knownKeys[key]; ok || strings.HasPrefix(strings.ToUpper(key), "OTEL_") {
			continue
		}
		return fmt.Errorf("unknown config key %q", key)
	}
	return nil
}

func bindTags(v *viper.Viper, iface any) error {
	ifv := reflect.ValueOf(iface)
	ift := reflect.TypeOf(iface)

	if ifv.Kind() == reflect.Pointer {
		ift = ift.Elem()
	}

	for field := range ift.Fields() {
		if envName, ok := field.Tag.Lookup("env"); ok {
			if err := v.BindEnv(envName, envName); err != nil {
				return fmt.Errorf("failed to bind env %s: %w", envName, err)
			}
		}
	}
	return nil
}

func exportEnv(cfg any) error {
	configValue := reflect.ValueOf(cfg).Elem()
	for field, value := range configValue.Fields() {
		name := field.Tag.Get("env")
		if name == "" {
			key := field.Tag.Get("mapstructure")
			if strings.HasPrefix(key, "OTEL_") {
				name = key
			}
		}
		if name == "" || value.Kind() != reflect.String || value.String() == "" {
			continue
		}
		if _, exists := os.LookupEnv(name); exists {
			continue
		}
		if err := os.Setenv(name, value.String()); err != nil {
			return fmt.Errorf("set %s: %w", name, err)
		}
	}
	return nil
}
