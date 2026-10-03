package config

import (
	"testing"
	"time"
)

func TestExampleConfig(t *testing.T) {
	c, err := Load("../../config.example.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if c.Decision.Provider != "mock" || c.LLM.Provider != "mock" {
		t.Errorf("example config should use mocks: %+v %+v", c.Decision, c.LLM)
	}
	if c.LLM.Timeout != 30*time.Second {
		t.Errorf("timeout = %v", c.LLM.Timeout)
	}
	if c.Interpretation.Clear != 0.8 || c.Interpretation.Unclear != 0.4 {
		t.Errorf("thresholds = %+v", c.Interpretation)
	}
}
