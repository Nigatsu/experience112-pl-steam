// Package platform wraps the few operating-system services the patcher
// needs: the registry (Steam location, Wine Direct3D settings), drive
// letters, GPU and Wine detection. On non-Windows systems the functions are
// inert stubs so the rest of the code can be built and tested anywhere.
package platform

// GPU describes the primary display adapter.
type GPU struct {
	Name   string
	NVIDIA bool
}

// WineD3DKey is the HKCU key holding Wine's Direct3D settings.
const WineD3DKey = `Software\Wine\Direct3D`
