package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/gorilla/mux"
)

type Route struct {
	Name        string
	Method      string
	Pattern     string
	HandlerFunc http.HandlerFunc
}

type Routes []Route

func NewRouter() *mux.Router {

	router := mux.NewRouter().StrictSlash(true)
	for _, route := range routes {
		router.
			Methods(route.Method).
			Path(route.Pattern).
			Name(route.Name).
			Handler(route.HandlerFunc)
	}

	return router
}

var routes = Routes{
	Route{
		"Index",
		"GET",
		"/",
		Index,
	},
	Route{
		"TodoIndex",
		"GET",
		"/api/v1/{method}",
		ApiMain,
	},
}

func Index(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintln(w, "Welcome!")
}

type myerror struct {
	Error string `json:"error"`
}

func ApiMain(w http.ResponseWriter, r *http.Request) {
	//	vars := mux.Vars(r)
	//	method := vars["method"]
	//	fmt.Fprintln(w, "Todo show:", method)
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	w.WriteHeader(http.StatusOK)

	var response ApiResponse

	if statsConn == nil || statsConn.CurrentTicketProfitability == nil {
		error := "Not Initialized Yet"
		response.Error = &error
		response.Result = nil
		response.Status = STATUS_NOT_INITIALIZED
	} else {
		response.Error = nil
		response.Result = statsConn.CurrentTicketProfitability
		response.Status = STATUS_OK
	}
	s, _ := json.Marshal(response)
	log.Printf("encoded as '%s'", s)

	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Fatal(err)
	}
}
