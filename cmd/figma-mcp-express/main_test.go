package main

import "testing"

func TestStdioRuntimeConfigFromEnvUsesDefaults(t *testing.T) {
	t.Setenv("FIGMA_MCP_STDIO_WORKERS", "")
	t.Setenv("FIGMA_MCP_STDIO_QUEUE", "")

	got := stdioRuntimeConfigFromEnv()

	if got.workerPoolSize != defaultStdioWorkerPoolSize {
		t.Fatalf("workerPoolSize = %d, want %d", got.workerPoolSize, defaultStdioWorkerPoolSize)
	}
	if got.queueSize != defaultStdioQueueSize {
		t.Fatalf("queueSize = %d, want %d", got.queueSize, defaultStdioQueueSize)
	}
}

func TestStdioRuntimeConfigFromEnvAcceptsPositiveValues(t *testing.T) {
	t.Setenv("FIGMA_MCP_STDIO_WORKERS", "32")
	t.Setenv("FIGMA_MCP_STDIO_QUEUE", "2048")

	got := stdioRuntimeConfigFromEnv()

	if got.workerPoolSize != 32 {
		t.Fatalf("workerPoolSize = %d, want 32", got.workerPoolSize)
	}
	if got.queueSize != 2048 {
		t.Fatalf("queueSize = %d, want 2048", got.queueSize)
	}
}

func TestStdioRuntimeConfigFromEnvRejectsInvalidValues(t *testing.T) {
	t.Setenv("FIGMA_MCP_STDIO_WORKERS", "0")
	t.Setenv("FIGMA_MCP_STDIO_QUEUE", "not-a-number")

	got := stdioRuntimeConfigFromEnv()

	if got.workerPoolSize != defaultStdioWorkerPoolSize {
		t.Fatalf("workerPoolSize = %d, want default %d", got.workerPoolSize, defaultStdioWorkerPoolSize)
	}
	if got.queueSize != defaultStdioQueueSize {
		t.Fatalf("queueSize = %d, want default %d", got.queueSize, defaultStdioQueueSize)
	}
}
