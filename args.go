package main

import "strings"

// splitArgs separates args into flag-tokens and positional-tokens, so flags
// can appear before, after, or interspersed with positional arguments —
// e.g. `find debug --current` and `find --current debug` both work.
//
// The stdlib "flag" package stops parsing at the first non-flag token,
// which otherwise silently swallows later flags as extra positional args.
// This function is tested directly (see args_test.go); main.go then calls
// fs.Parse(flagPart) and uses positional itself instead of fs.Args().
//
// stringFlagNames lists flags (without leading dashes) that consume the
// NEXT token as their value when written as "--flag value" (the "--flag=value"
// form is detected automatically and needs no special-casing). All other
// flags are treated as boolean (no following token consumed).
func splitArgs(args []string, stringFlagNames []string) (flagPart []string, positional []string) {
	isStringFlag := make(map[string]bool, len(stringFlagNames))
	for _, n := range stringFlagNames {
		isStringFlag[n] = true
	}

	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			positional = append(positional, args[i+1:]...)
			break
		}
		if strings.HasPrefix(a, "-") && a != "-" {
			flagPart = append(flagPart, a)
			name := strings.TrimLeft(a, "-")
			if !strings.Contains(name, "=") && isStringFlag[name] {
				if i+1 < len(args) {
					i++
					flagPart = append(flagPart, args[i])
				}
			}
			continue
		}
		positional = append(positional, a)
	}
	return flagPart, positional
}
