package main

/*
int add(int a, int b) {
    return a + b;
}
*/
import "C"

import "fmt"

func main() {
	fmt.Println("cgo ok:", C.add(3, 4))
}
