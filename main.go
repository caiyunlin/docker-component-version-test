package main

import (
	"fmt"

	"github.com/google/uuid"
)

func main() {
	id := uuid.New()
	fmt.Println("Hello from docker-component-version-test!")
	fmt.Printf("Generated UUID: %s\n", id)
}
