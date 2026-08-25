package fiberadapter_test

import (
	"fmt"

	fiberadapter "github.com/ashrafAli23/nestgo-fiber-adapter"
	core "github.com/ashrafAli23/nestgo/core"
)

func ExampleNew() {
	cfg := core.DefaultConfig()
	cfg.DisableLogger = true
	server := fiberadapter.New(cfg)
	server.GET("/hello", func(c core.Context) error {
		return c.JSON(200, map[string]string{"message": "hello"})
	})
	fmt.Println(server.Name())
	// Output: fiber
}
