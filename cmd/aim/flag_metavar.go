package main

import "github.com/spf13/pflag"

type stringMetavarValue struct {
	value    *string
	typeName string
}

func (v *stringMetavarValue) String() string {
	if v == nil || v.value == nil {
		return ""
	}
	return *v.value
}

func (v *stringMetavarValue) Set(value string) error {
	if v == nil || v.value == nil {
		return nil
	}
	*v.value = value
	return nil
}

func (v *stringMetavarValue) Type() string {
	return v.typeName
}

func stringFlagWithMetavar(fs *pflag.FlagSet, name, shorthand, defaultValue, usage, typeName string) {
	value := defaultValue
	flagValue := &stringMetavarValue{
		value:    &value,
		typeName: typeName,
	}
	if shorthand != "" {
		fs.VarP(flagValue, name, shorthand, usage)
		return
	}
	fs.Var(flagValue, name, usage)
}
