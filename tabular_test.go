package main

import (
	"encoding/csv"
	"io"
	"reflect"
	"strings"
	"testing"
)

func TestEmptyFile(t *testing.T) {
	_, err := NewTabularReader(strings.NewReader(""))
	if err != io.EOF {
		t.Fatal(err)
	}
}

func TestMalformedCsv(t *testing.T) {
	file := `foo,bar
a,b,c`
	_, err := NewTabularReader(strings.NewReader(file))
	if err == nil {
		t.Fatal("expected error but not found")
	}
	e, ok := err.(*csv.ParseError)
	if !ok {
		t.Fatalf("unexpected error type %#v", e)
	}
	if e.Err != csv.ErrFieldCount {
		t.Fatalf("expected field count error but found something else in %#v", e.Err)
	}
}

func TestMalformedTsv(t *testing.T) {
	file := `foo	bar
a	b	c`
	_, err := NewTabularReader(strings.NewReader(file))
	if err == nil {
		t.Fatal("expected error but not found")
	}
	e, ok := err.(*csv.ParseError)
	if !ok {
		t.Fatalf("unexpected error type %#v", e)
	}
	if e.Err != csv.ErrFieldCount {
		t.Fatalf("expected field count error but found something else in %#v", e.Err)
	}
}

func TestGoodCsv(t *testing.T) {
	file := `foo,bar
a,b
c,d
e,f
g,h`
	r, err := NewTabularReader(strings.NewReader(file))
	if err != nil {
		t.Fatal(err)
	}
	verify(t, r, map[string]string{"foo": "a", "bar": "b"}, nil)
	verify(t, r, map[string]string{"foo": "c", "bar": "d"}, nil)
	verify(t, r, map[string]string{"foo": "e", "bar": "f"}, nil)
	verify(t, r, map[string]string{"foo": "g", "bar": "h"}, nil)
	verify(t, r, nil, io.EOF)
}

func TestGoodTsv(t *testing.T) {
	file := `foo	bar
a	b
c	d
e	f
g	h`
	r, err := NewTabularReader(strings.NewReader(file))
	if err != nil {
		t.Fatal(err)
	}
	verify(t, r, map[string]string{"foo": "a", "bar": "b"}, nil)
	verify(t, r, map[string]string{"foo": "c", "bar": "d"}, nil)
	verify(t, r, map[string]string{"foo": "e", "bar": "f"}, nil)
	verify(t, r, map[string]string{"foo": "g", "bar": "h"}, nil)
	verify(t, r, nil, io.EOF)
}

func TestTsvContainingComma(t *testing.T) {
	file := `foo	bar
a	b,`
	r, err := NewTabularReader(strings.NewReader(file))
	if err != nil {
		t.Fatal(err)
	}
	verify(t, r, map[string]string{"foo": "a", "bar": "b,"}, nil)
	verify(t, r, nil, io.EOF)
}

func TestCsvContainingTab(t *testing.T) {
	file := `foo,bar
a,b	`
	r, err := NewTabularReader(strings.NewReader(file))
	if err != nil {
		t.Fatal(err)
	}
	verify(t, r, map[string]string{"foo": "a", "bar": "b\t"}, nil)
	verify(t, r, nil, io.EOF)
}

func verify(t *testing.T, reader *TabularReader, expectedRecord map[string]string, expectedErr error) {
	nextRecord, err := reader.Read()
	if err != expectedErr {
		t.Fatalf("Expected %#v but got %#v", expectedErr, err)
	}
	if !reflect.DeepEqual(nextRecord, expectedRecord) {
		t.Fatalf("Expected %#v but got %#v", expectedRecord, nextRecord)
	}
}
