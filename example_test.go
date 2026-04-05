package fiberadapter_test

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ashrafAli23/nestgo/core"
	fiber "github.com/ashrafAli23/nestgo-fiber-adapter"
)

func ExampleNew() {
	// Create a Fiber-backed NestGo server with default config.
	server := fiber.New(core.DefaultConfig())

	server.GET("/ping", func(c core.Context) error {
		return c.JSON(200, map[string]string{"pong": "true"})
	})

	// server.Start(":3000")
	fmt.Println("server type:", server.Name())
	// Output: server type: fiber
}

func ExampleNew_customConfig() {
	// Custom config with timeouts and body limit.
	config := &core.Config{
		AppName:      "my-api",
		Addr:         ":8080",
		ReadTimeout:  30,
		WriteTimeout: 30,
		BodyLimit:    10 * 1024 * 1024, // 10MB
	}

	server := fiber.New(config)
	fmt.Println("server type:", server.Name())
	// Output: server type: fiber
}

func ExampleFiberServer_Group() {
	server := fiber.New(core.DefaultConfig())

	// Create a route group with a prefix.
	api := server.Group("/api/v1")
	api.GET("/users", func(c core.Context) error {
		return c.JSON(200, []string{"alice", "bob"})
	})
	api.POST("/users", func(c core.Context) error {
		return c.JSON(201, map[string]string{"created": "true"})
	})

	fmt.Println("server type:", server.Name())
	// Output: server type: fiber
}

func ExampleFiberServer_Underlying() {
	server := fiber.New(core.DefaultConfig())

	// Access the raw *fiber.App for Fiber-specific features.
	// app := server.Underlying().(*fiber.App)
	// app.Static("/public", "./static")
	_ = server.Underlying()

	fmt.Println("underlying available:", server.Underlying() != nil)
	// Output: underlying available: true
}

func ExampleFiberServer_Shutdown() {
	server := fiber.New(core.DefaultConfig())

	// Graceful shutdown with OS signal handling.
	go func() {
		quit := make(chan os.Signal, 1)
		signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
		<-quit

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	}()

	// server.Start(":3000")
	fmt.Println("shutdown handler registered")
	// Output: shutdown handler registered
}

func ExampleReleaseSnapshot() {
	// ReleaseSnapshot returns a cloned context back to the pool.
	// This is optional — the GC handles cleanup if you don't call it.
	// But calling it reduces GC pressure in high-throughput apps.

	server := fiber.New(core.DefaultConfig())

	server.GET("/async", func(c core.Context) error {
		snapshot := c.Clone()
		go func() {
			defer fiber.ReleaseSnapshot(snapshot)
			// Safe to read request data from snapshot in goroutine.
			_ = snapshot.Method()
			_ = snapshot.Path()
			_ = snapshot.ClientIP()
		}()
		return c.JSON(202, map[string]string{"status": "accepted"})
	})

	fmt.Println("server type:", server.Name())
	// Output: server type: fiber
}
