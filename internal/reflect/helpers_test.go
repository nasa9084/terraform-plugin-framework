// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package reflect

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/hashicorp/terraform-plugin-framework/path"
)

type SimpleStruct struct {
	Field1 string `tfsdk:"field1"`
	Field2 int    `tfsdk:"field2"`
	Field3 bool   `tfsdk:"field3"`
	Field4 string `tfsdk:"-"`

	unexported          string //nolint:structcheck,unused
	unexportedAndTagged string `tfsdk:"unexported_and_tagged"`
}

type StructWithInvalidTag struct {
	InvalidField string `tfsdk:"*()-"`
}

func TestGetStructTags(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		in            any
		expectedTags  map[string][]int
		expectedError error
	}{
		"struct": {
			in: SimpleStruct{},
			expectedTags: map[string][]int{
				"field1": {0},
				"field2": {1},
				"field3": {2},
			},
		},
		"struct-err-duplicate-fields": {
			in: struct {
				Field1 string `tfsdk:"field1"`
				Field2 string `tfsdk:"field1"`
			}{},
			expectedError: errors.New(`field1: can't use field name for both Field1 and Field2`),
		},
		"struct-err-invalid-field": {
			in:            StructWithInvalidTag{},
			expectedError: errors.New(`*()-: invalid field name, must only use lowercase letters, underscores, and numbers, and must start with a letter`),
		},
		"embedded-struct": {
			in: struct {
				SimpleStruct
				Field5 string `tfsdk:"field5"`
			}{},
			expectedTags: map[string][]int{
				"field1": {0, 0},
				"field2": {0, 1},
				"field3": {0, 2},
				"field5": {1},
			},
		},
		"embedded-struct-unexported": {
			in: struct {
				SimpleStruct
				Field5 string `tfsdk:"field5"`

				unexported          string //nolint:structcheck,unused
				unexportedAndTagged string `tfsdk:"unexported_and_tagged"`
			}{},
			expectedTags: map[string][]int{
				"field1": {0, 0},
				"field2": {0, 1},
				"field3": {0, 2},
				"field5": {1},
			},
		},
		"embedded-struct-err-duplicate-fields": {
			in: struct {
				Field1       string `tfsdk:"field1"`
				SimpleStruct        // Contains a `tfsdk:"field1"`
			}{},
			expectedError: errors.New(`embedded struct "SimpleStruct" contains a duplicate field name "Field1"`),
		},
		"embedded-struct-err-invalid": {
			in: struct {
				StructWithInvalidTag // Contains an invalid "tfsdk" tag
			}{},
			expectedError: errors.New(`StructWithInvalidTag: failed to process embedded struct: *()-: invalid field name, must only use lowercase letters, underscores, and numbers, and must start with a letter`),
		},
		"embedded-struct-ptr": {
			in: struct {
				*SimpleStruct
				Field5 string `tfsdk:"field5"`
			}{},
			expectedTags: map[string][]int{
				"field1": {0, 0},
				"field2": {0, 1},
				"field3": {0, 2},
				"field5": {1},
			},
		},
		"embedded-struct-ptr-unexported": {
			in: struct {
				SimpleStruct
				Field5 string `tfsdk:"field5"`

				unexported          string //nolint:structcheck,unused
				unexportedAndTagged string `tfsdk:"unexported_and_tagged"`
			}{},
			expectedTags: map[string][]int{
				"field1": {0, 0},
				"field2": {0, 1},
				"field3": {0, 2},
				"field5": {1},
			},
		},
		"embedded-struct-ptr-err-duplicate-fields": {
			in: struct {
				Field1        string `tfsdk:"field1"`
				*SimpleStruct        // Contains a `tfsdk:"field1"`
			}{},
			expectedError: errors.New(`embedded struct "SimpleStruct" contains a duplicate field name "Field1"`),
		},
		"embedded-struct-ptr-err-invalid": {
			in: struct {
				*StructWithInvalidTag // Contains an invalid "tfsdk" tag
			}{},
			expectedError: errors.New(`StructWithInvalidTag: failed to process embedded struct: *()-: invalid field name, must only use lowercase letters, underscores, and numbers, and must start with a letter`),
		},
	}

	for name, testCase := range testCases {
		name, testCase := name, testCase
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			tags, err := getStructTags(context.Background(), reflect.TypeOf(testCase.in), path.Empty())
			if err != nil {
				if testCase.expectedError == nil {
					t.Fatalf("expected no error, got: %s", err)
				}

				if !strings.Contains(err.Error(), testCase.expectedError.Error()) {
					t.Fatalf("expected error %q, got: %s", testCase.expectedError, err)
				}
			}

			if err == nil && testCase.expectedError != nil {
				t.Fatalf("got no error, expected: %s", testCase.expectedError)
			}

			if diff := cmp.Diff(tags, testCase.expectedTags); diff != "" {
				t.Errorf("unexpected difference: %s", diff)
			}

		})
	}
}

func TestTrueReflectValue(t *testing.T) {
	t.Parallel()

	var iface, otherIface interface{}
	var stru struct{}

	// test that when nothing needs unwrapped, we get the right answer
	if got := trueReflectValue(reflect.ValueOf(stru)).Kind(); got != reflect.Struct {
		t.Errorf("Expected %s, got %s", reflect.Struct, got)
	}

	// test that we can unwrap pointers
	if got := trueReflectValue(reflect.ValueOf(&stru)).Kind(); got != reflect.Struct {
		t.Errorf("Expected %s, got %s", reflect.Struct, got)
	}

	// test that we can unwrap interfaces
	iface = stru
	if got := trueReflectValue(reflect.ValueOf(iface)).Kind(); got != reflect.Struct {
		t.Errorf("Expected %s, got %s", reflect.Struct, got)
	}

	// test that we can unwrap pointers inside interfaces, and pointers to
	// interfaces with pointers inside them
	iface = &stru
	if got := trueReflectValue(reflect.ValueOf(iface)).Kind(); got != reflect.Struct {
		t.Errorf("Expected %s, got %s", reflect.Struct, got)
	}
	if got := trueReflectValue(reflect.ValueOf(&iface)).Kind(); got != reflect.Struct {
		t.Errorf("Expected %s, got %s", reflect.Struct, got)
	}

	// test that we can unwrap pointers to interfaces inside other
	// interfaces, and pointers to interfaces inside pointers to
	// interfaces.
	otherIface = &iface
	if got := trueReflectValue(reflect.ValueOf(otherIface)).Kind(); got != reflect.Struct {
		t.Errorf("Expected %s, got %s", reflect.Struct, got)
	}
	if got := trueReflectValue(reflect.ValueOf(&otherIface)).Kind(); got != reflect.Struct {
		t.Errorf("Expected %s, got %s", reflect.Struct, got)
	}
}

func TestCommaSeparatedString(t *testing.T) {
	t.Parallel()
	type testCase struct {
		input    []string
		expected string
	}
	tests := map[string]testCase{
		"empty": {
			input:    []string{},
			expected: "",
		},
		"oneWord": {
			input:    []string{"red"},
			expected: "red",
		},
		"twoWords": {
			input:    []string{"red", "blue"},
			expected: "red and blue",
		},
		"threeWords": {
			input:    []string{"red", "blue", "green"},
			expected: "red, blue, and green",
		},
		"fourWords": {
			input:    []string{"red", "blue", "green", "purple"},
			expected: "red, blue, green, and purple",
		},
	}
	for name, test := range tests {
		name, test := name, test
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got := commaSeparatedString(test.input)
			if got != test.expected {
				t.Errorf("Expected %q, got %q", test.expected, got)
			}
		})
	}
}

func TestIsValidFieldName(t *testing.T) {
	t.Parallel()
	tests := map[string]bool{
		"":    false,
		"a":   true,
		"1":   false,
		"1a":  false,
		"a1":  true,
		"A":   false,
		"a-b": false,
		"a_b": true,
	}
	for in, expected := range tests {
		in, expected := in, expected
		t.Run(fmt.Sprintf("input=%q", in), func(t *testing.T) {
			t.Parallel()

			result := isValidFieldName(in)
			if result != expected {
				t.Errorf("Expected %v, got %v", expected, result)
			}
		})
	}
}

func TestCanBeNil_struct(t *testing.T) {
	t.Parallel()

	var stru struct{}

	got := canBeNil(reflect.ValueOf(stru))
	if got {
		t.Error("Expected structs to not be nillable, but canBeNil said they were")
	}
}

func TestCanBeNil_structPointer(t *testing.T) {
	t.Parallel()

	var stru struct{}
	struPtr := &stru

	got := canBeNil(reflect.ValueOf(struPtr))
	if !got {
		t.Error("Expected pointers to structs to be nillable, but canBeNil said they weren't")
	}
}

func TestCanBeNil_slice(t *testing.T) {
	t.Parallel()

	slice := []string{}
	got := canBeNil(reflect.ValueOf(slice))
	if !got {
		t.Errorf("Expected slices to be nillable, but canBeNil said they weren't")
	}
}

func TestCanBeNil_map(t *testing.T) {
	t.Parallel()

	m := map[string]string{}
	got := canBeNil(reflect.ValueOf(m))
	if !got {
		t.Errorf("Expected maps to be nillable, but canBeNil said they weren't")
	}
}

func TestCanBeNil_interface(t *testing.T) {
	t.Parallel()

	type myStruct struct {
		Value interface{}
	}

	var s myStruct
	got := canBeNil(reflect.ValueOf(s).FieldByName("Value"))
	if !got {
		t.Errorf("Expected interfaces to be nillable, but canBeNil said they weren't")
	}
}
