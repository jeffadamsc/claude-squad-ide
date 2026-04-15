package app

import (
	"testing"
)

func TestExtractGoSymbols(t *testing.T) {
	src := []byte(`package main

// MyFunc does something.
func MyFunc(x int) error {
    return nil
}

type MyStruct struct {
    Name string
}

func (m *MyStruct) Method() {}
`)

	symbols, refs, err := extractSymbols("test.go", src)
	if err != nil {
		t.Fatal(err)
	}

	// Should find: MyFunc, MyStruct, Method
	names := make(map[string]bool)
	for _, s := range symbols {
		names[s.Name] = true
	}

	for _, want := range []string{"MyFunc", "MyStruct", "Method"} {
		if !names[want] {
			t.Errorf("missing symbol %q", want)
		}
	}

	// refs is for call graph - not empty for Method call would be ideal
	_ = refs
}

func TestExtractTypeScriptSymbols(t *testing.T) {
	src := []byte(`
interface User {
    name: string;
}

class UserService {
    getUser(): User {
        return { name: "test" };
    }
}

function helper(x: number): void {}
`)

	symbols, _, err := extractSymbols("test.ts", src)
	if err != nil {
		t.Fatal(err)
	}

	names := make(map[string]bool)
	for _, s := range symbols {
		names[s.Name] = true
	}

	for _, want := range []string{"User", "UserService", "getUser", "helper"} {
		if !names[want] {
			t.Errorf("missing symbol %q", want)
		}
	}
}

func TestExtractPythonSymbols(t *testing.T) {
	src := []byte(`
class MyClass:
    def method(self):
        pass

def my_function(x):
    return x * 2
`)

	symbols, _, err := extractSymbols("test.py", src)
	if err != nil {
		t.Fatal(err)
	}

	names := make(map[string]bool)
	for _, s := range symbols {
		names[s.Name] = true
	}

	for _, want := range []string{"MyClass", "method", "my_function"} {
		if !names[want] {
			t.Errorf("missing symbol %q", want)
		}
	}
}

func TestExtractJavaScriptSymbols(t *testing.T) {
	src := []byte(`
function greet(name) {
    return "Hello, " + name;
}

class Calculator {
    add(a, b) {
        return a + b;
    }
}

const helper = (x) => x * 2;
`)

	symbols, _, err := extractSymbols("test.js", src)
	if err != nil {
		t.Fatal(err)
	}

	names := make(map[string]bool)
	for _, s := range symbols {
		names[s.Name] = true
	}

	for _, want := range []string{"greet", "Calculator", "add"} {
		if !names[want] {
			t.Errorf("missing symbol %q", want)
		}
	}
}

func TestUnsupportedExtension(t *testing.T) {
	symbols, refs, err := extractSymbols("test.xyz", []byte("random content"))
	if err != nil {
		t.Fatal(err)
	}
	if len(symbols) != 0 || len(refs) != 0 {
		t.Error("unsupported extensions should return empty results")
	}
}
