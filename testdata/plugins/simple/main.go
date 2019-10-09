package main

import "io"

var (
	String    = "string"
	Pointer   = &String
	Interface = io.Reader(nil)
)

func Function() string { return "function" }
