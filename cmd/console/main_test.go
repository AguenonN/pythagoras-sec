package main

import (
	"reflect"
	"testing"
)

func TestChoisirCompilateur(t *testing.T) {
	avecCle := func(k string) string {
		if k == "MISTRAL_API_KEY" {
			return "x"
		}
		return ""
	}
	sansCle := func(string) string { return "" }
	for _, c := range []struct {
		valeur, force string
		env           func(string) string
		cmd           []string
		mode          string
	}{
		{"", "", avecCle, nil, "reel"},
		{"", "", sansCle, nil, "rejeu"},
		{"rejeu", "", avecCle, nil, "rejeu"},
		{"reel", "", sansCle, nil, "reel"},
		{"python3 sim.py", "", avecCle, []string{"python3", "sim.py"}, "reel"},
		{"python3 sim.py", "rejeu", avecCle, []string{"python3", "sim.py"}, "rejeu"},
	} {
		cmd, mode, err := choisirCompilateur(c.valeur, c.force, c.env)
		if err != nil || mode != c.mode || len(cmd)+len(c.cmd) > 0 && !reflect.DeepEqual(cmd, c.cmd) {
			t.Errorf("%q %q : %v %q %v, attendu %v %q", c.valeur, c.force, cmd, mode, err, c.cmd, c.mode)
		}
	}
	if _, _, err := choisirCompilateur("", "vrai", sansCle); err == nil {
		t.Error("mode inconnu accepté")
	}
}
