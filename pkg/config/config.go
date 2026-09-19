package config

import (
	"errors"
	"fmt"
	"os"
	"reflect"

	"github.com/go-playground/validator/v10"
	"github.com/spf13/viper"
)

func New(cfg any) error {
	return newWithPaths(cfg, []string{"config.yaml", "config.local.yaml"})
}

func newWithPaths(cfg any, paths []string) error {
	v := viper.New()

	if err := bindTags(v, cfg); err != nil {
		return err
	}

	v.SetConfigType("yaml")
	for _, path := range paths {
		v.SetConfigFile(path)
		if err := v.MergeInConfig(); err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				return err
			}
		}
	}

	if err := v.UnmarshalExact(cfg); err != nil {
		return err
	}

	if err := validator.New().Struct(cfg); err != nil {
		return err
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
