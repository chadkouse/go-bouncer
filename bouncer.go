package bouncer

import (
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"strings"
)

const (
	jsonContentType           = "application/json; charset=utf-8"
	StatusUnprocessableEntity = 422
)

type BouncerHandler struct {
	iface interface{}
	f     http.Handler
}

func NewBouncerHandler(obj interface{}, f http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := BouncerHandler{
			f:     f,
			iface: obj,
		}
		h.ServeHTTP(w, r)
	})
}

func (h BouncerHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {

	errs := Validate(h.iface, r)

	if len(errs) > 0 {
		ErrorHandler(errs, w)
		return
	}

	h.f.ServeHTTP(w, r)

}

// ErrorHandler simply counts the number of errors in the
// context and, if more than 0, writes a response with an
// error code and a JSON payload describing the errors.
// The response will have a JSON content-type.
// Middleware remaining on the stack will not even see the request
// if, by this point, there are any errors.
// This is a "default" handler, of sorts, and you are
// welcome to use your own instead. The Bind middleware
// invokes this automatically for convenience.
func ErrorHandler(errs Errors, resp http.ResponseWriter) {
	if len(errs) > 0 {
		resp.Header().Set("Content-Type", jsonContentType)
		if errs.Has(DeserializationError) {
			resp.WriteHeader(http.StatusBadRequest)
		} else if errs.Has(ContentTypeError) {
			resp.WriteHeader(http.StatusUnsupportedMediaType)
		} else {
			resp.WriteHeader(StatusUnprocessableEntity)
		}
		errOutput, _ := json.Marshal(errs)
		resp.Write(errOutput)
		return
	}
}

func Validate(obj interface{}, req *http.Request) Errors {
	contentType := req.Header.Get("Content-Type")
	if req.Method == "POST" || req.Method == "PUT" || req.Method == "PATCH" || contentType != "" {

		if strings.Contains(contentType, "json") {
			return Json(obj, req)
		}
		return nil
	}
	return nil
}

func Json(jsonStruct interface{}, req *http.Request) Errors {

	var errors Errors
	ensureNotPointer(jsonStruct)
	obj := reflect.New(reflect.TypeOf(jsonStruct))

	if req.Body != nil {
		err := json.NewDecoder(req.Body).Decode(obj.Interface())
		if err != nil && err != io.EOF {
			errors.Add([]string{}, DeserializationError, err.Error())
		}
	}

	if req.Method == "PATCH" {
		errors = validatePatchStruct(errors, obj.Interface())
	} else if req.Method == "POST" || req.Method == "PUT" {
		errors = validateCreateStruct(errors, obj.Interface())
	}

	return errors
}

func validateCreateStruct(errors Errors, obj interface{}) Errors {
	typ := reflect.TypeOf(obj)
	val := reflect.ValueOf(obj)

	if typ.Kind() == reflect.Ptr {
		typ = typ.Elem()
		val = val.Elem()
	}

	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)

		// Skip ignored and unexported fields in the struct
		if field.Tag.Get("form") == "-" || !val.Field(i).CanInterface() {
			continue
		}

		fieldValue := val.Field(i).Interface()
		zero := reflect.Zero(field.Type).Interface()

		// Validate nested and embedded structs (if pointer, only do so if not nil)
		if field.Type.Kind() == reflect.Struct ||
			(field.Type.Kind() == reflect.Ptr && !reflect.DeepEqual(zero, fieldValue) &&
				field.Type.Elem().Kind() == reflect.Struct) {
			errors = validateCreateStruct(errors, fieldValue)
		}

		if field.Tag.Get("create") == "-" {
			//this is immutable - make sure it's zero
			if !reflect.DeepEqual(zero, fieldValue) {
				name := field.Name
				if j := field.Tag.Get("json"); j != "" {
					name = j
				} else if f := field.Tag.Get("form"); f != "" {
					name = f
				}
				errors.Add([]string{name}, ImmutableError, "Immutable")
			}
		}

		if strings.Index(field.Tag.Get("create"), "required") > -1 {
			if reflect.DeepEqual(zero, fieldValue) {
				name := field.Name
				if j := field.Tag.Get("json"); j != "" {
					name = j
				} else if f := field.Tag.Get("form"); f != "" {
					name = f
				}
				errors.Add([]string{name}, RequiredError, "Required")
			}
		}
	}
	return errors

}

func validatePatchStruct(errors Errors, obj interface{}) Errors {
	typ := reflect.TypeOf(obj)
	val := reflect.ValueOf(obj)

	if typ.Kind() == reflect.Ptr {
		typ = typ.Elem()
		val = val.Elem()
	}

	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)

		// Skip ignored and unexported fields in the struct
		if field.Tag.Get("form") == "-" || !val.Field(i).CanInterface() {
			continue
		}

		fieldValue := val.Field(i).Interface()
		zero := reflect.Zero(field.Type).Interface()

		// Validate nested and embedded structs (if pointer, only do so if not nil)
		if field.Type.Kind() == reflect.Struct ||
			(field.Type.Kind() == reflect.Ptr && !reflect.DeepEqual(zero, fieldValue) &&
				field.Type.Elem().Kind() == reflect.Struct) {
			errors = validatePatchStruct(errors, fieldValue)
		}

		if field.Tag.Get("patch") == "-" {
			//this is immutable - make sure it's zero
			if !reflect.DeepEqual(zero, fieldValue) {
				name := field.Name
				if j := field.Tag.Get("json"); j != "" {
					name = j
				} else if f := field.Tag.Get("form"); f != "" {
					name = f
				}
				errors.Add([]string{name}, ImmutableError, "Immutable")
			}
		}
	}
	return errors

}

// Don't pass in pointers to bouncer
func ensureNotPointer(obj interface{}) {
	if reflect.TypeOf(obj).Kind() == reflect.Ptr {
		panic("Pointers are not accepted as binding models")
	}
}