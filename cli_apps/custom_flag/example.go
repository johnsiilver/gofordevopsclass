package main

import (
	"flag"
	"fmt"
	"net/url"
	"os"
	"reflect"
)

// URLValue is a custom flag.Value that parses a URL. It implements the flag.Value interface.
type URLValue struct {
	URL *url.URL
}

// String returns the string representation of the URL.
func (v URLValue) String() string {
	if v.URL != nil {
		return v.URL.String()
	}
	return ""
}

// Set parses the flag text and stores it in the URLValue.
func (v URLValue) Set(s string) error {
	if u, err := url.Parse(s); err != nil {
		return err
	} else {
		*v.URL = *u
	}
	return nil
}

var u = &url.URL{}

func init() {
	flag.Var(&URLValue{u}, "url", "URL to parse")
}

func main() {
	flag.Parse()
	if reflect.ValueOf(*u).IsZero() {
		fmt.Printf("did not pass an URL\n")
		os.Exit(1)
	}
	fmt.Printf("{scheme: %q, host: %q, path: %q}\n", u.Scheme, u.Host, u.Path)
}
