package v1

import (
	"reflect"
	"testing"
)

// TestNoAnonymousStructFields guards against https://github.com/monzo/egress-operator/issues/49:
// controller-gen panics (v0.16-v0.21) or errors (v0.22+) when a CRD field has an inline
// struct type, so every struct-typed field in this package's API types must use a named type.
func TestNoAnonymousStructFields(t *testing.T) {
	pkg := reflect.TypeOf(ExternalService{}).PkgPath()
	seen := map[reflect.Type]bool{}

	var walk func(typ reflect.Type, path string)
	walk = func(typ reflect.Type, path string) {
		for typ.Kind() == reflect.Ptr || typ.Kind() == reflect.Slice || typ.Kind() == reflect.Array || typ.Kind() == reflect.Map {
			typ = typ.Elem()
		}
		if typ.Kind() != reflect.Struct {
			return
		}
		if typ.Name() == "" {
			t.Errorf("%s has an anonymous struct type; declare a named type instead (see issue #49)", path)
		} else if typ.PkgPath() != pkg || seen[typ] {
			// Only our own API types are generated from this package; k8s types are already valid.
			return
		}
		seen[typ] = true
		for i := 0; i < typ.NumField(); i++ {
			f := typ.Field(i)
			walk(f.Type, path+"."+f.Name)
		}
	}

	walk(reflect.TypeOf(ExternalService{}), "ExternalService")
	walk(reflect.TypeOf(ExternalServiceList{}), "ExternalServiceList")
}
