package gate

import (
	"reflect"
	"strings"
	"testing"
)

func TestIdentifiants(t *testing.T) {
	cas := []struct {
		args    map[string]any
		ptr     string
		attendu []string
		lisible bool
	}{
		{map[string]any{"id": "A"}, "/id", []string{"A"}, true},
		{map[string]any{"ids": []any{"A", "B", "A", 12.0, 1.5, ""}}, "/ids", []string{"A", "B", "12"}, true},
		{map[string]any{}, "/id", nil, true},
		{map[string]any{"id": nil}, "/id", nil, true},
		{map[string]any{"a": map[string]any{"b~c": []any{"X"}}}, "/a/b~0c", []string{"X"}, true},
		{map[string]any{"ids": make([]any, SujetsMax+1)}, "/ids", nil, false},
	}
	for i, c := range cas {
		got, ok := identifiants(c.args, c.ptr)
		if ok != c.lisible || !reflect.DeepEqual(got, c.attendu) {
			t.Errorf("cas %d : %v %v", i, got, ok)
		}
	}
	if _, ok := pointer(map[string]any{}, "sans-barre"); ok {
		t.Fatal("pointeur sans / accepté")
	}
	if id, ok := identifiant(1e21); !ok || strings.Contains(id, "e") {
		t.Fatalf("entier en notation exponentielle : %q", id)
	}
}
