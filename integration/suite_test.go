//go:build integration

package integration_test

import (
	"testing"

	"github.com/JaimeStill/capstone-bunrei/integration"
)

func TestMain(m *testing.M) { integration.Main(m) }
