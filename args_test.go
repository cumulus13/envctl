package main

import (
	"reflect"
	"testing"
)

func TestSplitArgs(t *testing.T) {
	cases := []struct {
		name           string
		args           []string
		stringFlags    []string
		wantFlags      []string
		wantPositional []string
	}{
		{
			name:           "flag after positional (the reported bug)",
			args:           []string{"debug", "--current"},
			stringFlags:    []string{"config"},
			wantFlags:      []string{"--current"},
			wantPositional: []string{"debug"},
		},
		{
			name:           "flag before positional",
			args:           []string{"--current", "debug"},
			stringFlags:    []string{"config"},
			wantFlags:      []string{"--current"},
			wantPositional: []string{"debug"},
		},
		{
			name:           "wildcard pattern with flag after",
			args:           []string{"*debug*", "--current"},
			stringFlags:    []string{"config"},
			wantFlags:      []string{"--current"},
			wantPositional: []string{"*debug*"},
		},
		{
			name:           "string flag with separate value, interspersed",
			args:           []string{"debug", "--config", "C:\\x.json", "--current"},
			stringFlags:    []string{"config"},
			wantFlags:      []string{"--config", "C:\\x.json", "--current"},
			wantPositional: []string{"debug"},
		},
		{
			name:           "string flag with = form",
			args:           []string{"debug", "--config=C:\\x.json"},
			stringFlags:    []string{"config"},
			wantFlags:      []string{"--config=C:\\x.json"},
			wantPositional: []string{"debug"},
		},
		{
			name:           "set: two positionals with flag after both",
			args:           []string{"MY_VAR", "C:\\tools", "--user"},
			stringFlags:    []string{"type"},
			wantFlags:      []string{"--user"},
			wantPositional: []string{"MY_VAR", "C:\\tools"},
		},
		{
			name:           "set: flag sandwiched between positionals",
			args:           []string{"MY_VAR", "--user", "C:\\tools"},
			stringFlags:    []string{"type"},
			wantFlags:      []string{"--user"},
			wantPositional: []string{"MY_VAR", "C:\\tools"},
		},
		{
			name:           "-- stops flag parsing entirely",
			args:           []string{"--current", "--", "-literally-a-pattern"},
			stringFlags:    []string{"config"},
			wantFlags:      []string{"--current"},
			wantPositional: []string{"-literally-a-pattern"},
		},
		{
			name:           "empty args",
			args:           []string{},
			stringFlags:    []string{"config"},
			wantFlags:      nil,
			wantPositional: nil,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			gotFlags, gotPositional := splitArgs(c.args, c.stringFlags)
			if !reflect.DeepEqual(gotFlags, c.wantFlags) {
				t.Errorf("flags: got %#v, want %#v", gotFlags, c.wantFlags)
			}
			if !reflect.DeepEqual(gotPositional, c.wantPositional) {
				t.Errorf("positional: got %#v, want %#v", gotPositional, c.wantPositional)
			}
		})
	}
}
