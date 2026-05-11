//go:build ignore

package main

import (
	"fmt"
	"reflect"
)

type MoveTo struct {
	X, Y float64
}

type LineTo struct {
	X, Y float64
}

type Close struct{}

func main() {
	moveTo := &MoveTo{X: 100, Y: 100}
	lineTo := &LineTo{X: 200, Y: 100}
	closePath := &Close{}
	
	elements := []interface{}{moveTo, lineTo, closePath}
	
	for _, elem := range elements {
		val := reflect.ValueOf(elem)
		fmt.Printf("\nOriginal: kind=%v, type=%v\n", val.Kind(), val.Type())
		
		if val.Kind() == reflect.Ptr {
			val = val.Elem()
			typeName := val.Type().Name()
			fullTypeName := val.Type().String()
			fmt.Printf("After deref: kind=%v, typeName=%v, fullName=%v\n", val.Kind(), typeName, fullTypeName)
		}
	}
}
