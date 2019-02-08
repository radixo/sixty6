package sixty6

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"unicode"
)

type MiddlewareFunction func(*HttpHandler) bool

type internalHandler struct {
	userHandlerType reflect.Type
	middlewares []MiddlewareFunction
	pattern string
}

type httpHandler interface {
	GetHttpHandler(interface{}) *HttpHandler
	Params(string) interface{}
	BodyMap() map[string]interface{}
}

type HttpHandler struct {
	bodyMap map[string]interface{}
	sessions map[string]map[string]interface{}
	contentType string
	response []byte
	MethodName string
	Res http.ResponseWriter
	Req *http.Request
}

func HttpHandle(pattern string, userHandler httpHandler,
    middlewares ...MiddlewareFunction) {
	var ih = &internalHandler{ userHandlerType: reflect.TypeOf(userHandler),
	    middlewares: middlewares, pattern: pattern }

	// Register http.Handler
	http.Handle(pattern, ih)
}

func ServeFile(name string) (func (http.ResponseWriter, *http.Request)) {

	return func (w http.ResponseWriter, r *http.Request) {

		http.ServeFile(w, r, name)
		return
	}
}

func (h *HttpHandler) Serror(serr string, status int) {

	h.Error(errors.New(serr), status)
}

func (h *HttpHandler) Error(err error, status int) {

	http.Error(h.Res, err.Error(), status)
}

func (h *HttpHandler) GetHttpHandler(instance interface{}) *HttpHandler {

	if h == nil {
		newh := new(HttpHandler)
		v := reflect.ValueOf(instance).Elem().FieldByName("HttpHandler")
		v.Set(reflect.ValueOf(newh))
		return newh
	}
	return h
}

func (h *HttpHandler) SessionByName(name string) map[string]interface{} {
	var err error

	if h.sessions == nil {
		h.sessions = make(map[string]map[string]interface{})
	}

	if h.sessions[name] != nil {
		return h.sessions[name]
	}

	h.sessions[name], err = GetCookie(h.Req, name)
	if err != nil {
		h.sessions[name] = make(map[string]interface{})
	}

	return h.sessions[name]
}

func (h *HttpHandler) Session() map[string]interface{} {

	return h.SessionByName("session")
}

func (h *HttpHandler) Params(key string) interface{} {

	val, ok := h.Req.Form[key]
	if !ok {
		return nil
	}

	return val
}

func (h *HttpHandler) BodyMap() map[string]interface{} {
	var v interface{}

	if h.bodyMap != nil {
		return h.bodyMap
	}

	h.bodyMap = make(map[string]interface{})

	// Parse json
	if len(h.Req.Header["Content-Type"]) > 0 &&
	    strings.ToLower(h.Req.Header["Content-Type"][0]) ==
	    "application/json" {
		buf := new(bytes.Buffer)
		buf.ReadFrom(h.Req.Body)
		if err := json.Unmarshal(buf.Bytes(), &v); err == nil {
			switch val := v.(type) {
			case map[string]interface{}:
				h.bodyMap = val
			default:
				h.bodyMap[""] = val
			}
			return h.bodyMap
		}
	}

	// Parse PostForm
	for key, val := range h.Req.PostForm {
		if len(val) > 1 {
			h.bodyMap[key] = val
		} else {
			h.bodyMap[key] = val[0]
		}
	}

	return h.bodyMap
}

func (h *HttpHandler) JsonResponse(obj interface{}) error {

	ret, err := json.Marshal(obj)
	if err != nil {
		return err
	}

	h.contentType = "application/json"
	h.response = ret
	return nil
}

func parsePath(userHandler interface{}, hh *HttpHandler, h *internalHandler) (
    reflect.Value, string) {

	// Check method to be used
	subtree := hh.Req.URL.Path[len(h.pattern):]
	methodb := []byte(strings.ToLower(hh.Req.Method))
	methodb[0] = strings.ToUpper(string(methodb[0]))[0]
	pos := strings.Index(subtree, "/")
	action := ""
	if len(hh.Req.URL.Path) > len(h.pattern) {
		if pos > -1 {
			action = subtree[:pos]
		} else {
			action = subtree
		}
		first := true
		action = strings.Map(func(r rune) rune {
			switch {
			case r == '-':
				first = true
				return -1
			case first:
				first = false
				return unicode.ToUpper(r)
			default:
				return r
			}
		}, action)
	}
	method := string(methodb)

	if action != "" {
		action += method
		m := reflect.ValueOf(userHandler).MethodByName(action)
		if (m.IsValid()) {
			hh.MethodName = action
			if pos > -1 {
				return m, subtree[pos:]
			} else {
				return m, ""
			}
		}
	}

	m := reflect.ValueOf(userHandler).MethodByName(method)
	if (m.IsValid()) {
		hh.MethodName = method
		return m, subtree
	}

	m = reflect.ValueOf(userHandler).MethodByName("Default")
	if (m.IsValid()) {
		hh.MethodName = "Default"
		return m, subtree
	}

	return reflect.ValueOf(nil), ""
}

func (h *internalHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var userHandler = reflect.New(h.userHandlerType).Interface()
	var httpHandler = userHandler.(httpHandler).GetHttpHandler(userHandler)
	var method reflect.Value
	var urlParams string

	// Set Response and Request
	httpHandler.Res = w
	httpHandler.Req = r

	method, urlParams = parsePath(userHandler, httpHandler, h)
	if urlParams == "XXX" {
		panic("Got the easter egg!")
	}

	// If has no methods
	if method.IsValid() == false {
		http.NotFound(w, r)
		return
	}

	// Read forms and ignore errors
	_ = r.ParseMultipartForm(1048576) // 1 MB

	// Read json doc

	// Pass by middlewares
	for _, m := range h.middlewares {
		if m(httpHandler) == false {
			return
		}
	}

	// Call the method dynamically
	method.Call([]reflect.Value{})

	// Set sessions cookies
	for name, sess := range httpHandler.sessions {
		if len(sess) == 0 {
			http.SetCookie(w, &http.Cookie{ Name: name })
		} else if err := SetCookie(w, &http.Cookie{ Name: name }, sess);
		    err != nil {
			    panic(err)
		}
	}

	// Write Response
	if httpHandler.response != nil {
		w.Header().Set("Content-Type", httpHandler.contentType)
		w.Write(httpHandler.response)
	}

	return
}
