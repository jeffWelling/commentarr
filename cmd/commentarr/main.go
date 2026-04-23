// Command commentarr is the Commentarr binary. Plan 1 ships only the
// scan subcommand; later plans add serve, migrate, and so on.
package main

import (
	"fmt"

	_ "github.com/jeffWelling/commentary-classifier"
)

func main() {
	fmt.Println("commentarr (skeleton)")
}
