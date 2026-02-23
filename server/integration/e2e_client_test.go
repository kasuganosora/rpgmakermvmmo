//go:build e2e

package integration

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestClientServerE2E starts the Go integration test server, then runs
// the Node.js E2E test suite that loads actual client plugin JS against it.
//
// Usage: go test -tags=e2e ./integration/ -v -count=1 -timeout 120s
func TestClientServerE2E(t *testing.T) {
	// Check that node is available.
	nodePath, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not found in PATH, skipping client E2E tests")
	}
	t.Logf("Using node: %s", nodePath)

	// Locate the client_plugin/tests directory.
	testsDir := findTestsDir(t)
	t.Logf("Tests dir: %s", testsDir)

	// npm install if node_modules doesn't exist.
	ensureNpmInstall(t, testsDir)

	// Start the real integration test server (in-memory DB, random port).
	ts := NewTestServer(t)
	defer ts.Close()
	t.Logf("Server URL: %s", ts.URL)
	t.Logf("WS URL:     %s", ts.WSURL)

	// Run the Node.js test runner.
	cmd := exec.Command(nodePath, "run-all.js", ts.URL, ts.WSURL)
	cmd.Dir = testsDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(),
		"TEST_HTTP_URL="+ts.URL,
		"TEST_WS_URL="+ts.WSURL,
	)

	err = cmd.Run()
	require.NoError(t, err, "client E2E tests failed")
}

// findTestsDir locates the mmo/client_plugin/tests directory relative to this file.
func findTestsDir(t *testing.T) string {
	t.Helper()

	// This file is in mmo/server/integration/
	// The tests are in mmo/client_plugin/tests/
	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok, "runtime.Caller failed")

	integrationDir := filepath.Dir(thisFile)
	serverDir := filepath.Dir(integrationDir)
	mmoDir := filepath.Dir(serverDir)
	testsDir := filepath.Join(mmoDir, "client_plugin", "tests")

	_, err := os.Stat(testsDir)
	require.NoError(t, err, "client_plugin/tests directory not found at %s", testsDir)
	return testsDir
}

// ensureNpmInstall runs `npm install` in the tests directory if node_modules is missing.
func ensureNpmInstall(t *testing.T, testsDir string) {
	t.Helper()
	nodeModules := filepath.Join(testsDir, "node_modules")
	if _, err := os.Stat(nodeModules); err == nil {
		return // already installed
	}

	npmPath, err := exec.LookPath("npm")
	if err != nil {
		t.Fatal("npm not found in PATH; run 'npm install' manually in " + testsDir)
	}

	t.Log("Running npm install...")
	cmd := exec.Command(npmPath, "install")
	cmd.Dir = testsDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	require.NoError(t, cmd.Run(), "npm install failed")
}
