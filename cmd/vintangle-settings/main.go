package main

import (
	"fmt"

	"github.com/diamondburned/gotk4/pkg/gio/v2"
)

func main() {
	settings := gio.NewSettings("com.pojtinger.felicitas.vintangle.state")

	fmt.Println(settings.Int64("verbose"))

	settings.SetInt64("verbose", 0)

	fmt.Println(settings.Int64("verbose"))
}
