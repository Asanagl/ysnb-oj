//go:build !linux

package main

import "fmt"

func main() { fmt.Println("probe requires linux build") }
