package main

import (
	"fmt"
	"strings"
)

type namedValue map[string]string

func (nv namedValue) String() string {
	return fmt.Sprint(map[string]string(nv))
}

func (nv namedValue) Set(value string) error {
	tokens := strings.SplitN(value, "=", 2)
	if len(tokens) != 2 {
		return fmt.Errorf("Unable to parse [%v], it should be in the format of key=value", value)
	}
	nv[tokens[0]] = tokens[1]
	return nil
}
