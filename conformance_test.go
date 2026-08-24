package fiberadapter_test

import (
	"testing"

	fiberadapter "github.com/ashrafAli23/nestgo-fiber-adapter"
	"github.com/ashrafAli23/nestgo/conformance"
	core "github.com/ashrafAli23/nestgo/core"
)

func TestConformance(t *testing.T) {
	conformance.Run(t, func(cfg *core.Config) core.Server {
		return fiberadapter.New(cfg)
	})
}
