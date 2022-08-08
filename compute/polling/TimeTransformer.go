package polling

import (
	"reflect"
	"time"
)

//Transformer is an addition from https://pkg.go.dev/github.com/imdario/mergo@v0.3.13 and supplements mergo with a handling for zero time stamps
type timeTransformer struct {
}

func (t *timeTransformer) Transformer(typ reflect.Type) func(dst, src reflect.Value) error {
	if typ == reflect.TypeOf(time.Time{}) {
		return func(dst, src reflect.Value) error {
			if dst.CanSet() {
				isZero := dst.MethodByName("IsZero")
				result := isZero.Call([]reflect.Value{})
				if result[0].Bool() {
					dst.Set(src)
				}
			}
			return nil
		}
	}
	return nil
}
